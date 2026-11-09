package services

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func TestCreateUploadSignature_SignsExpectedParams(t *testing.T) {
	t.Setenv("CLOUDINARY_URL", "cloudinary://test-key:test-secret@demo-cloud")
	service := &postService{}
	userID := uuid.NewString()

	signature, err := service.CreateUploadSignature(context.Background(), userID, "auto", "")
	if err != nil {
		t.Fatalf("CreateUploadSignature: %v", err)
	}

	if signature.CloudName != "demo-cloud" || signature.APIKey != "test-key" {
		t.Fatalf("credentials = %+v", signature)
	}
	if signature.Folder != "socialflow/"+userID {
		t.Errorf("folder = %q", signature.Folder)
	}
	if signature.PublicID == "" {
		t.Error("public id is empty")
	}
	if signature.UploadURL != "https://api.cloudinary.com/v1_1/demo-cloud/auto/upload" {
		t.Errorf("upload url = %q", signature.UploadURL)
	}
	if signature.ExpiresAt != signature.Timestamp+600 {
		t.Errorf("expiry = %d, timestamp = %d", signature.ExpiresAt, signature.Timestamp)
	}

	sum := sha1.Sum([]byte(fmt.Sprintf(
		"folder=%s&public_id=%s&timestamp=%d%s",
		signature.Folder, signature.PublicID, signature.Timestamp, "test-secret",
	)))
	if want := hex.EncodeToString(sum[:]); signature.Signature != want {
		t.Fatalf("signature = %q, want %q", signature.Signature, want)
	}
}

func TestCreateUploadSignature_RejectsInvalidInput(t *testing.T) {
	t.Setenv("CLOUDINARY_URL", "cloudinary://test-key:test-secret@demo-cloud")
	service := &postService{}

	if _, err := service.CreateUploadSignature(context.Background(), "not-a-uuid", "auto", ""); err == nil {
		t.Fatal("expected invalid user id error")
	}
	if _, err := service.CreateUploadSignature(context.Background(), uuid.NewString(), "executable", ""); err == nil {
		t.Fatal("expected unsupported resource type error")
	}
}
