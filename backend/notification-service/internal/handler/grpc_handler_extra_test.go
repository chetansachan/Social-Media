package handler

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	notificationpb "github.com/chetansachan/socialflow/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMarkAllAsReadReturnsTheCount(t *testing.T) {
	service := &stubNotificationService{markedAll: 12}
	h := NewGRPCHandler(service, nil)

	res, err := h.MarkAllAsRead(context.Background(), &notificationpb.MarkAllAsReadRequest{UserId: uuid.NewString()})
	if err != nil {
		t.Fatalf("MarkAllAsRead: %v", err)
	}
	if res.GetMarkedCount() != 12 {
		t.Fatalf("marked = %d, want 12", res.GetMarkedCount())
	}
}

func TestDeleteNotificationIsScopedToItsUser(t *testing.T) {
	service := &stubNotificationService{}
	h := NewGRPCHandler(service, nil)

	userID := uuid.New()
	if _, err := h.DeleteNotification(context.Background(), &notificationpb.DeleteNotificationRequest{
		UserId:         userID.String(),
		NotificationId: uuid.NewString(),
	}); err != nil {
		t.Fatalf("DeleteNotification: %v", err)
	}

	service.err = models.ErrNotificationNotFound
	if _, err := h.DeleteNotification(context.Background(), &notificationpb.DeleteNotificationRequest{
		UserId:         userID.String(),
		NotificationId: uuid.NewString(),
	}); status.Code(err) != codes.NotFound {
		t.Fatalf("a notification belonging to someone else must read as not found, got %v", status.Code(err))
	}
}

func TestDeviceTokenRPCSucceed(t *testing.T) {
	h := NewGRPCHandler(&stubNotificationService{}, nil)
	userID := uuid.New().String()

	registered, err := h.RegisterDeviceToken(context.Background(), &notificationpb.RegisterDeviceTokenRequest{
		UserId:     userID,
		Token:      "device-token",
		Platform:   "android",
		DeviceName: "pixel",
		AppVersion: "1.2.3",
	})
	if err != nil {
		t.Fatalf("RegisterDeviceToken: %v", err)
	}
	if !registered.GetSuccess() {
		t.Fatal("register should report success")
	}

	revoked, err := h.RevokeDeviceToken(context.Background(), &notificationpb.RevokeDeviceTokenRequest{
		UserId: userID,
		Token:  "device-token",
	})
	if err != nil {
		t.Fatalf("RevokeDeviceToken: %v", err)
	}
	if !revoked.GetSuccess() {
		t.Fatal("revoke should report success")
	}
}

// An invalid token is a client error, so it must not look like an outage.
func TestDeviceTokenRPCsMapValidationErrors(t *testing.T) {
	h := NewGRPCHandler(&stubNotificationService{err: models.ErrInvalidInput}, nil)

	_, err := h.RegisterDeviceToken(context.Background(), &notificationpb.RegisterDeviceTokenRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("register: code = %v, want InvalidArgument", status.Code(err))
	}

	_, err = h.RevokeDeviceToken(context.Background(), &notificationpb.RevokeDeviceTokenRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("revoke: code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestFromProtoSettingsIsNilSafe(t *testing.T) {
	userID := uuid.New()

	if settings := fromProtoSettings(userID, nil); settings != nil {
		t.Fatalf("nil proto mapped to %+v, want nil", settings)
	}

	proto := &notificationpb.NotificationSettings{
		InappLikes:      true,
		PushEnabled:     true,
		EmailEnabled:    true,
		QuietHoursStart: "22:00",
		QuietHoursEnd:   "07:00",
		Timezone:        "Asia/Kolkata",
	}
	settings := fromProtoSettings(userID, proto)
	if settings.UserID != userID {
		t.Fatalf("user = %s, want %s", settings.UserID, userID)
	}
	if !settings.InAppLikes || !settings.PushEnabled || !settings.EmailEnabled {
		t.Fatalf("settings = %+v", settings)
	}
	if settings.QuietHoursStart != "22:00" || settings.Timezone != "Asia/Kolkata" {
		t.Fatalf("quiet hours/zone = %+v", settings)
	}
}

func TestDecodeJWTPartAcceptsBothBase64Forms(t *testing.T) {
	raw := []byte(`{"alg":"HS256"}`)

	padded := base64.RawURLEncoding.EncodeToString(raw)
	decoded, err := decodeJWTPart(padded)
	if err != nil {
		t.Fatalf("decode raw base64url: %v", err)
	}
	if string(decoded) != string(raw) {
		t.Fatalf("decoded = %s, want %s", decoded, raw)
	}

	// A header produced by standard Base64URL with padding must still decode:
	// the two libraries differ on their padding, and both tokens are in the wild.
	withPadding := base64.URLEncoding.EncodeToString(raw)
	decoded, err = decodeJWTPart(withPadding)
	if err != nil {
		t.Fatalf("decode padded base64url: %v", err)
	}
	if string(decoded) != string(raw) {
		t.Fatalf("decoded = %s, want %s", decoded, raw)
	}

	if _, err := decodeJWTPart("not base64 at all"); err == nil {
		t.Fatal("a malformed segment must be rejected")
	}
	if _, err := decodeJWTPart(""); err == nil {
		t.Fatal("an empty segment must be rejected")
	}
}
