package handler

import (
	"context"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/chetansachan/socialflow/notification-service/internal/services"
	notificationpb "github.com/chetansachan/socialflow/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type stubNotificationService struct {
	services.NotificationService

	page      *models.NotificationPage
	settings  *models.NotificationSettings
	unread    int32
	marked    int32
	markedAll int32
	err       error
}

func (s *stubNotificationService) GetNotifications(ctx context.Context, userID string, pageSize int32, cursor, filter string) (*models.NotificationPage, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.page, nil
}

func (s *stubNotificationService) GetUnreadCount(ctx context.Context, userID string) (int32, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.unread, nil
}

func (s *stubNotificationService) MarkAsRead(ctx context.Context, userID string, ids []string) (int32, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.marked, nil
}

func (s *stubNotificationService) MarkAllAsRead(ctx context.Context, userID string) (int32, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.markedAll, nil
}

func (s *stubNotificationService) DeleteNotification(ctx context.Context, userID, notificationID string) error {
	return s.err
}

func (s *stubNotificationService) GetSettings(ctx context.Context, userID string) (*models.NotificationSettings, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.settings, nil
}

func (s *stubNotificationService) UpdateSettings(ctx context.Context, userID string, settings *models.NotificationSettings) (*models.NotificationSettings, error) {
	if s.err != nil {
		return nil, s.err
	}
	return settings, nil
}

func (s *stubNotificationService) RegisterDeviceToken(ctx context.Context, userID, token, platform, deviceName, appVersion string) error {
	return s.err
}

func (s *stubNotificationService) RevokeDeviceToken(ctx context.Context, userID, token string) error {
	return s.err
}

func TestGetNotificationsMapsItems(t *testing.T) {
	t.Parallel()

	recipient := uuid.New()
	actorID := uuid.NewString()
	createdAt := time.Now().UTC()

	service := &stubNotificationService{
		page: &models.NotificationPage{
			Items: []models.Notification{
				{
					ID:               uuid.New(),
					RecipientID:      recipient,
					Type:             models.TypePostLike,
					EntityID:         "post-1",
					EntityType:       "post",
					ActorIDs:         []string{actorID},
					Actors:           []models.Actor{{ID: actorID, DisplayName: "Alice"}},
					TotalActorsCount: 4,
					Title:            "Alice and 3 others liked your post",
					ContentPreview:   "nice",
					ActionURL:        "socialflow://posts/post-1",
					CreatedAt:        createdAt,
					UpdatedAt:        createdAt,
				},
			},
			NextCursor:  "next",
			HasMore:     true,
			UnreadCount: 7,
		},
	}

	handler := NewGRPCHandler(service, nil)
	response, err := handler.GetNotifications(context.Background(), &notificationpb.GetNotificationsRequest{
		UserId:     recipient.String(),
		PageSize:   10,
		FilterType: notificationpb.NotificationType_NOTIFICATION_TYPE_POST_LIKE,
	})
	if err != nil {
		t.Fatalf("GetNotifications: %v", err)
	}

	if len(response.GetNotifications()) != 1 {
		t.Fatalf("items = %d, want 1", len(response.GetNotifications()))
	}
	item := response.GetNotifications()[0]
	if item.GetType() != notificationpb.NotificationType_NOTIFICATION_TYPE_POST_LIKE {
		t.Errorf("type = %v", item.GetType())
	}
	if item.GetTotalActorsCount() != 4 {
		t.Errorf("actor count = %d", item.GetTotalActorsCount())
	}
	if len(item.GetActors()) != 1 || item.GetActors()[0].GetDisplayName() != "Alice" {
		t.Errorf("actors = %+v", item.GetActors())
	}
	if response.GetUnreadCount() != 7 || !response.GetHasMore() || response.GetNextCursor() != "next" {
		t.Errorf("page metadata = %+v", response)
	}
}

func TestMarkAsReadRequiresIDs(t *testing.T) {
	t.Parallel()

	handler := NewGRPCHandler(&stubNotificationService{}, nil)
	_, err := handler.MarkAsRead(context.Background(), &notificationpb.MarkAsReadRequest{UserId: uuid.NewString()})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestGRPCErrorMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err  error
		code codes.Code
	}{
		{models.ErrInvalidInput, codes.InvalidArgument},
		{models.ErrInvalidCursor, codes.InvalidArgument},
		{models.ErrNotificationNotFound, codes.NotFound},
		{context.DeadlineExceeded, codes.DeadlineExceeded},
	}

	for _, tc := range cases {
		handler := NewGRPCHandler(&stubNotificationService{err: tc.err}, nil)
		_, err := handler.GetUnreadCount(context.Background(), &notificationpb.GetUnreadCountRequest{UserId: uuid.NewString()})
		if status.Code(err) != tc.code {
			t.Errorf("error %v mapped to %v, want %v", tc.err, status.Code(err), tc.code)
		}
	}
}

func TestUpdateSettingsRequiresPayload(t *testing.T) {
	t.Parallel()

	handler := NewGRPCHandler(&stubNotificationService{}, nil)
	_, err := handler.UpdateSettings(context.Background(), &notificationpb.UpdateSettingsRequest{UserId: uuid.NewString()})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	settings := models.DefaultSettings(userID)
	settings.PushLikes = true
	settings.QuietHoursEnabled = true
	settings.QuietHoursStart = "22:00"
	settings.QuietHoursEnd = "07:00"
	settings.Timezone = "Asia/Kolkata"

	handler := NewGRPCHandler(&stubNotificationService{settings: settings}, nil)
	response, err := handler.GetSettings(context.Background(), &notificationpb.GetSettingsRequest{UserId: userID.String()})
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if !response.GetSettings().GetPushLikes() || response.GetSettings().GetQuietHoursStart() != "22:00" {
		t.Fatalf("settings did not round trip: %+v", response.GetSettings())
	}
}

func TestToProtoType(t *testing.T) {
	t.Parallel()

	if got := toProtoType(models.TypeSystemAlert); got != notificationpb.NotificationType_NOTIFICATION_TYPE_SYSTEM_ALERT {
		t.Errorf("system alert mapping = %v", got)
	}
	if got := toProtoType(models.NotificationType("unknown")); got != notificationpb.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED {
		t.Errorf("unknown mapping = %v", got)
	}
}
