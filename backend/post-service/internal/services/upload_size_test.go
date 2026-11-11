package services

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/chetansachan/socialflow/post-service/internal/config"
	"github.com/chetansachan/socialflow/post-service/internal/models"
)

// dataURI builds the payload a browser (or curl) would actually send.
func dataURI(mimeType string, size int64) string {
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(make([]byte, size))
}

// enforce runs the production check against the config the test just set up
// with t.Setenv, so a test can exercise a custom limit as easily as a default.
func enforce(t *testing.T, payload, mimeType string) error {
	t.Helper()
	return enforceMediaSize(config.Load(), payload, mimeType)
}

func TestMediaSizeLimitsDefault(t *testing.T) {
	cfg := config.Load()

	if cfg.MediaImageMaxBytes != config.DefaultMediaImageMaxBytes {
		t.Errorf("image limit = %d, want %d", cfg.MediaImageMaxBytes, config.DefaultMediaImageMaxBytes)
	}
	if cfg.MediaVideoMaxBytes != config.DefaultMediaVideoMaxBytes {
		t.Errorf("video limit = %d, want %d", cfg.MediaVideoMaxBytes, config.DefaultMediaVideoMaxBytes)
	}

	// Video gets the larger wall: a clip is the same kind of attachment but
	// carries far more bytes per second of content.
	if cfg.MediaVideoMaxBytes <= cfg.MediaImageMaxBytes {
		t.Fatal("video should allow more bytes than an image")
	}
}

func TestMediaSizeLimitsAreConfigurable(t *testing.T) {
	t.Setenv("MEDIA_IMAGE_MAX_BYTES", "2048")
	t.Setenv("MEDIA_VIDEO_MAX_BYTES", "4096")

	cfg := config.Load()
	if cfg.MediaImageMaxBytes != 2048 {
		t.Errorf("image limit = %d, want 2048", cfg.MediaImageMaxBytes)
	}
	if cfg.MediaVideoMaxBytes != 4096 {
		t.Errorf("video limit = %d, want 4096", cfg.MediaVideoMaxBytes)
	}
}

func TestEnforceMediaSizeRejectsOversizedImage(t *testing.T) {
	payload := dataURI("image/png", config.DefaultMediaImageMaxBytes+1)

	err := enforce(t, payload, "image/png")
	if err == nil {
		t.Fatal("expected an oversized image to be rejected")
	}
	// The handler maps ErrInvalidInput to InvalidArgument, so a caller gets a
	// 400 with a reason rather than a 500.
	if !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("error should wrap ErrInvalidInput so it becomes a 400: %v", err)
	}
	if !strings.Contains(err.Error(), "10 MB") {
		t.Errorf("message should name the cap: %v", err)
	}
}

func TestEnforceMediaSizeRejectsOversizedVideo(t *testing.T) {
	payload := dataURI("video/mp4", config.DefaultMediaVideoMaxBytes+1)

	err := enforce(t, payload, "video/mp4")
	if err == nil {
		t.Fatal("expected an oversized video to be rejected")
	}
	if !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("error should wrap ErrInvalidInput: %v", err)
	}
	if !strings.Contains(err.Error(), "64 MB") {
		t.Errorf("message should name the cap: %v", err)
	}
}

func TestEnforceMediaSizeAllowsWithinLimits(t *testing.T) {
	// The boundary itself is allowed: "up to 10 MB" means 10 MB is fine.
	within := []struct {
		mime string
		size int64
	}{
		{"image/png", config.DefaultMediaImageMaxBytes},
		{"image/gif", config.DefaultMediaImageMaxBytes},
		{"video/mp4", config.DefaultMediaVideoMaxBytes},
		{"video/webm", config.DefaultMediaVideoMaxBytes},
	}

	for _, tc := range within {
		if err := enforce(t, dataURI(tc.mime, tc.size), tc.mime); err != nil {
			t.Errorf("%s at %d bytes should be allowed, got %v", tc.mime, tc.size, err)
		}
	}
}

// The cap is measured on decoded bytes, not on the encoded length. Base64 is a
// third larger, so measuring the encoded form would reject files the policy
// allows.
func TestEnforceMediaSizeMeasuresDecodedBytes(t *testing.T) {
	payload := dataURI("image/png", config.DefaultMediaImageMaxBytes)

	if len(payload) <= config.DefaultMediaImageMaxBytes {
		t.Skip("base64 overhead is not larger than the payload at this size")
	}
	if err := enforce(t, payload, "image/png"); err != nil {
		t.Fatalf("a payload whose encoded form exceeds the limit but whose decoded form does not should be allowed: %v", err)
	}
}

// A URL is already hosted on Cloudinary, and its size is only knowable after a
// fetch this service never performs. Capping it here would reject every file
// the composer legitimately uploads through the signed path.
func TestEnforceMediaSizeIgnoresHostedURLs(t *testing.T) {
	urls := []string{
		"https://res.cloudinary.com/demo/image/upload/v1/socialflow/u1/abc.png",
		"http://example.test/a-very-long-path-that-is-not-really-a-file.bin",
	}

	for _, url := range urls {
		if err := enforce(t, url, "image/png"); err != nil {
			t.Errorf("%s should not be size-checked: %v", url, err)
		}
	}
}

// A body that is not decodable is not a size problem. Reporting it as one would
// send the caller chasing the wrong limit.
func TestEnforceMediaSizeIgnoresUnmeasurablePayloads(t *testing.T) {
	if err := enforce(t, "data:image/png;base64,not-valid-base64!!", "image/png"); err != nil {
		t.Fatalf("an undecodable payload should not be reported as oversized: %v", err)
	}
	if err := enforce(t, "", ""); err != nil {
		t.Fatalf("an empty payload should not be reported as oversized: %v", err)
	}
}

func TestEnforceMediaSizeUsesTheKindFromTheMIMEType(t *testing.T) {
	// The same byte count is over the image cap but under the video cap, so the
	// outcome is decided entirely by which kind the MIME type names.
	var size int64 = config.DefaultMediaImageMaxBytes + 1024

	if err := enforce(t, dataURI("image/png", size), "image/png"); err == nil {
		t.Fatal("expected the image cap to apply to image/png")
	}
	// The MIME type is what routes the decision, so a video prefix is allowed
	// the same bytes.
	if err := enforce(t, dataURI("video/mp4", size), "video/mp4"); err != nil {
		t.Fatalf("expected the video cap to allow the same bytes: %v", err)
	}
}
