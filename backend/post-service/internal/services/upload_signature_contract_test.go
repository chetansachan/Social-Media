package services

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/chetansachan/socialflow/post-service/internal/models"
	"github.com/google/uuid"
)

/* ---- The upload contract -------------------------------------------------
   A direct Cloudinary upload is signed by post-service and executed by the
   browser. Those are two programs that have to agree exactly, and the failure
   mode is the expensive one: the file uploads in full and only then does
   Cloudinary reject the signature.

   Cloudinary's rule is that the signature covers every parameter in the request
   except api_key, resource_type, file, cloud_name and signature. So the browser
   may send exactly the fields post-service signed -- and adding one, even
   harmless-looking like a width, breaks it.

   These tests assert both halves of that without reaching Cloudinary. */

// cloudinaryFormFields mirrors the FormData keys uploadToCloudinary sends in
// frontend/src/lib/media.ts. Duplicating the list here is the point: it is a
// contract between two codebases, and a test that derives it from one side would
// only ever agree with itself.
var cloudinaryFormFields = []string{
	"file",
	"folder",
	"public_id",
	"timestamp",
	"api_key",
	"signature",
}

// cloudinaryExcluded are the parameters Cloudinary leaves out of its own check.
// Everything else in the body must be in the signature.
var cloudinaryExcluded = map[string]bool{
	"api_key":       true,
	"resource_type": true,
	"file":          true,
	"cloud_name":    true,
	"signature":     true,
}

// cloudinaryVerify recomputes the signature the way Cloudinary does: sort the
// signed parameters, join them as key=value with '&', append the secret, and
// SHA-1 the result.
func cloudinaryVerify(form url.Values, secret string) string {
	keys := make([]string, 0, len(form))
	for key := range form {
		if cloudinaryExcluded[key] {
			continue
		}
		// Cloudinary ignores a parameter sent with no value.
		if strings.TrimSpace(form.Get(key)) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+form.Get(key))
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "&") + secret))
	return hex.EncodeToString(sum[:])
}

// buildForm constructs the request the browser sends from a signature.
func buildForm(signature *models.UploadSignature) url.Values {
	form := url.Values{}
	form.Set("file", "data:image/png;base64,iVBORw0KGgoAAAANSUhEUg==")
	for _, key := range cloudinaryFormFields {
		switch key {
		case "folder":
			form.Set(key, signature.Folder)
		case "public_id":
			form.Set(key, signature.PublicID)
		case "timestamp":
			form.Set(key, fmt.Sprintf("%d", signature.Timestamp))
		case "api_key":
			form.Set(key, signature.APIKey)
		case "signature":
			form.Set(key, signature.Signature)
		}
	}
	return form
}

func TestBrowserUploadFormSatisfiesTheSignature(t *testing.T) {
	const secret = "test-secret"
	t.Setenv("CLOUDINARY_URL", "cloudinary://test-key:"+secret+"@demo-cloud")

	service := &postService{}
	userID := uuid.NewString()

	signature, err := service.CreateUploadSignature(context.Background(), userID, "image", "")
	if err != nil {
		t.Fatalf("CreateUploadSignature: %v", err)
	}

	form := buildForm(signature)

	// The browser must not send a field post-service did not sign.
	if len(form) != len(cloudinaryFormFields) {
		t.Fatalf("form has %d fields, want %d: %v", len(form), len(cloudinaryFormFields), form)
	}
	for _, key := range cloudinaryFormFields {
		if !form.Has(key) {
			t.Errorf("form is missing %q", key)
		}
	}

	if got, want := cloudinaryVerify(form, secret), signature.Signature; got != want {
		t.Fatalf(
			"Cloudinary would reject this upload:\n  form signature    = %s\n  post-service gave = %s",
			got, want,
		)
	}
}

// The reason the field list above is load-bearing: one extra parameter, however
// innocent, changes the signed string and 400s the whole file.
func TestAddingAnyFieldBreaksTheSignature(t *testing.T) {
	const secret = "test-secret"
	t.Setenv("CLOUDINARY_URL", "cloudinary://test-key:"+secret+"@demo-cloud")

	service := &postService{}
	signature, err := service.CreateUploadSignature(context.Background(), uuid.NewString(), "image", "")
	if err != nil {
		t.Fatalf("CreateUploadSignature: %v", err)
	}

	// Each of these is a thing someone would plausibly want to add, and each
	// would silently invalidate the signature.
	for _, extra := range []string{"width", "eager", "format", "tags", "invalidate"} {
		form := buildForm(signature)
		form.Set(extra, "600")

		if got := cloudinaryVerify(form, secret); got == signature.Signature {
			t.Errorf(
				"adding %q did not change the signature; that field would 400 at Cloudinary and the test would not notice",
				extra,
			)
		}
	}
}

// resource_type lives in the URL path, not the body, so it never reaches the
// signature check -- which is why the gateway can pick image or video without
// changing what the browser signs.
func TestResourceTypeIsNotInTheSignedBody(t *testing.T) {
	const secret = "test-secret"
	t.Setenv("CLOUDINARY_URL", "cloudinary://test-key:"+secret+"@demo-cloud")

	service := &postService{}
	imageSignature, err := service.CreateUploadSignature(context.Background(), uuid.NewString(), "image", "")
	if err != nil {
		t.Fatalf("CreateUploadSignature(image): %v", err)
	}
	videoSignature, err := service.CreateUploadSignature(context.Background(), uuid.NewString(), "video", "")
	if err != nil {
		t.Fatalf("CreateUploadSignature(video): %v", err)
	}

	if imageSignature.UploadURL != "https://api.cloudinary.com/v1_1/demo-cloud/image/upload" {
		t.Errorf("image upload url = %q", imageSignature.UploadURL)
	}
	if videoSignature.UploadURL != "https://api.cloudinary.com/v1_1/demo-cloud/video/upload" {
		t.Errorf("video upload url = %q", videoSignature.UploadURL)
	}

	// The signed part is the same three fields either way, so switching kinds
	// does not change the contract.
	if got := cloudinaryVerify(buildForm(imageSignature), secret); got != imageSignature.Signature {
		t.Errorf("image signature does not match its form")
	}
	if got := cloudinaryVerify(buildForm(videoSignature), secret); got != videoSignature.Signature {
		t.Errorf("video signature does not match its form")
	}
}
