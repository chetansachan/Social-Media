package handler

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/chetansachan/socialflow/notification-service/internal/services"
	notificationpb "github.com/chetansachan/socialflow/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCHandler implements notification.NotificationServiceServer.
type GRPCHandler struct {
	notificationpb.UnimplementedNotificationServiceServer
	service services.NotificationService
	logger  *log.Logger
}

func NewGRPCHandler(service services.NotificationService, logger *log.Logger) *GRPCHandler {
	if logger == nil {
		logger = log.Default()
	}
	return &GRPCHandler{service: service, logger: logger}
}

func (h *GRPCHandler) GetNotifications(ctx context.Context, req *notificationpb.GetNotificationsRequest) (*notificationpb.GetNotificationsResponse, error) {
	filter := ""
	if req.GetFilterType() != notificationpb.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED {
		filter = strings.ToLower(strings.TrimPrefix(req.GetFilterType().String(), "NOTIFICATION_TYPE_"))
	}

	page, err := h.service.GetNotifications(ctx, req.GetUserId(), req.GetPageSize(), req.GetCursor(), filter)
	if err != nil {
		return nil, h.mapError("GetNotifications", err)
	}

	items := make([]*notificationpb.NotificationItem, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, toProtoNotification(&page.Items[i]))
	}

	return &notificationpb.GetNotificationsResponse{
		Notifications: items,
		NextCursor:    page.NextCursor,
		HasMore:       page.HasMore,
		UnreadCount:   page.UnreadCount,
	}, nil
}

func (h *GRPCHandler) GetUnreadCount(ctx context.Context, req *notificationpb.GetUnreadCountRequest) (*notificationpb.GetUnreadCountResponse, error) {
	count, err := h.service.GetUnreadCount(ctx, req.GetUserId())
	if err != nil {
		return nil, h.mapError("GetUnreadCount", err)
	}
	return &notificationpb.GetUnreadCountResponse{UnreadCount: count}, nil
}

func (h *GRPCHandler) MarkAsRead(ctx context.Context, req *notificationpb.MarkAsReadRequest) (*notificationpb.MarkAsReadResponse, error) {
	if len(req.GetNotificationIds()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "notification_ids must not be empty")
	}

	remaining, err := h.service.MarkAsRead(ctx, req.GetUserId(), req.GetNotificationIds())
	if err != nil {
		return nil, h.mapError("MarkAsRead", err)
	}
	return &notificationpb.MarkAsReadResponse{
		Success:              true,
		RemainingUnreadCount: remaining,
	}, nil
}

func (h *GRPCHandler) MarkAllAsRead(ctx context.Context, req *notificationpb.MarkAllAsReadRequest) (*notificationpb.MarkAllAsReadResponse, error) {
	marked, err := h.service.MarkAllAsRead(ctx, req.GetUserId())
	if err != nil {
		return nil, h.mapError("MarkAllAsRead", err)
	}
	return &notificationpb.MarkAllAsReadResponse{
		Success:     true,
		MarkedCount: marked,
	}, nil
}

func (h *GRPCHandler) DeleteNotification(ctx context.Context, req *notificationpb.DeleteNotificationRequest) (*notificationpb.DeleteNotificationResponse, error) {
	if err := h.service.DeleteNotification(ctx, req.GetUserId(), req.GetNotificationId()); err != nil {
		return nil, h.mapError("DeleteNotification", err)
	}
	return &notificationpb.DeleteNotificationResponse{Success: true}, nil
}

func (h *GRPCHandler) GetSettings(ctx context.Context, req *notificationpb.GetSettingsRequest) (*notificationpb.GetSettingsResponse, error) {
	settings, err := h.service.GetSettings(ctx, req.GetUserId())
	if err != nil {
		return nil, h.mapError("GetSettings", err)
	}
	return &notificationpb.GetSettingsResponse{Settings: toProtoSettings(settings)}, nil
}

func (h *GRPCHandler) UpdateSettings(ctx context.Context, req *notificationpb.UpdateSettingsRequest) (*notificationpb.UpdateSettingsResponse, error) {
	if req.GetSettings() == nil {
		return nil, status.Error(codes.InvalidArgument, "settings payload is required")
	}

	userID, err := uuid.Parse(strings.TrimSpace(req.GetUserId()))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}

	settings := fromProtoSettings(userID, req.GetSettings())
	updated, err := h.service.UpdateSettings(ctx, req.GetUserId(), settings)
	if err != nil {
		return nil, h.mapError("UpdateSettings", err)
	}

	return &notificationpb.UpdateSettingsResponse{
		Success:  true,
		Settings: toProtoSettings(updated),
	}, nil
}

func (h *GRPCHandler) RegisterDeviceToken(ctx context.Context, req *notificationpb.RegisterDeviceTokenRequest) (*notificationpb.RegisterDeviceTokenResponse, error) {
	if err := h.service.RegisterDeviceToken(ctx, req.GetUserId(), req.GetToken(), req.GetPlatform(), req.GetDeviceName(), req.GetAppVersion()); err != nil {
		return nil, h.mapError("RegisterDeviceToken", err)
	}
	return &notificationpb.RegisterDeviceTokenResponse{Success: true}, nil
}

func (h *GRPCHandler) RevokeDeviceToken(ctx context.Context, req *notificationpb.RevokeDeviceTokenRequest) (*notificationpb.RevokeDeviceTokenResponse, error) {
	if err := h.service.RevokeDeviceToken(ctx, req.GetUserId(), req.GetToken()); err != nil {
		return nil, h.mapError("RevokeDeviceToken", err)
	}
	return &notificationpb.RevokeDeviceTokenResponse{Success: true}, nil
}

func (h *GRPCHandler) mapError(operation string, err error) error {
	switch {
	case errors.Is(err, models.ErrInvalidInput), errors.Is(err, models.ErrInvalidCursor):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, models.ErrNotificationNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "request timed out")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request cancelled")
	default:
		h.logger.Printf("%s failed: %v", operation, err)
		return status.Error(codes.Internal, "internal server error")
	}
}

func toProtoNotification(notification *models.Notification) *notificationpb.NotificationItem {
	actors := make([]*notificationpb.NotificationActor, 0, len(notification.Actors))
	for _, actor := range notification.Actors {
		actors = append(actors, &notificationpb.NotificationActor{
			Id:          actor.ID,
			Username:    actor.Username,
			DisplayName: actor.DisplayName,
			AvatarUrl:   actor.AvatarURL,
		})
	}

	return &notificationpb.NotificationItem{
		Id:               notification.ID.String(),
		RecipientId:      notification.RecipientID.String(),
		Type:             toProtoType(notification.Type),
		EntityId:         notification.EntityID,
		EntityType:       notification.EntityType,
		Actors:           actors,
		TotalActorsCount: int32(notification.TotalActorsCount),
		Title:            notification.Title,
		ContentPreview:   notification.ContentPreview,
		ActionUrl:        notification.ActionURL,
		IsRead:           notification.IsRead,
		CreatedAt:        notification.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        notification.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toProtoType(notificationType models.NotificationType) notificationpb.NotificationType {
	switch notificationType {
	case models.TypeFollow:
		return notificationpb.NotificationType_NOTIFICATION_TYPE_FOLLOW
	case models.TypePostLike:
		return notificationpb.NotificationType_NOTIFICATION_TYPE_POST_LIKE
	case models.TypeComment:
		return notificationpb.NotificationType_NOTIFICATION_TYPE_COMMENT
	case models.TypeCommentReply:
		return notificationpb.NotificationType_NOTIFICATION_TYPE_COMMENT_REPLY
	case models.TypeMention:
		return notificationpb.NotificationType_NOTIFICATION_TYPE_MENTION
	case models.TypeCreatorPost:
		return notificationpb.NotificationType_NOTIFICATION_TYPE_CREATOR_POST
	case models.TypeSystemAlert:
		return notificationpb.NotificationType_NOTIFICATION_TYPE_SYSTEM_ALERT
	default:
		return notificationpb.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED
	}
}

func toProtoSettings(settings *models.NotificationSettings) *notificationpb.NotificationSettings {
	if settings == nil {
		return &notificationpb.NotificationSettings{}
	}
	return &notificationpb.NotificationSettings{
		InappLikes:          settings.InAppLikes,
		InappComments:       settings.InAppComments,
		InappFollows:        settings.InAppFollows,
		InappMentions:       settings.InAppMentions,
		PushEnabled:         settings.PushEnabled,
		PushLikes:           settings.PushLikes,
		PushComments:        settings.PushComments,
		PushFollows:         settings.PushFollows,
		PushMentions:        settings.PushMentions,
		EmailEnabled:        settings.EmailEnabled,
		EmailWeeklyDigest:   settings.EmailWeeklyDigest,
		EmailSecurityAlerts: settings.EmailSecurityAlerts,
		QuietHoursEnabled:   settings.QuietHoursEnabled,
		QuietHoursStart:     settings.QuietHoursStart,
		QuietHoursEnd:       settings.QuietHoursEnd,
		Timezone:            settings.Timezone,
	}
}

// fromProtoSettings maps a client payload. A nil settings message has no
// meaning rather than "turn everything off", so it maps to nil rather than a
// struct of zero values that a caller could mistake for a real choice.
func fromProtoSettings(userID uuid.UUID, settings *notificationpb.NotificationSettings) *models.NotificationSettings {
	if settings == nil {
		return nil
	}
	return &models.NotificationSettings{
		UserID:              userID,
		InAppLikes:          settings.GetInappLikes(),
		InAppComments:       settings.GetInappComments(),
		InAppFollows:        settings.GetInappFollows(),
		InAppMentions:       settings.GetInappMentions(),
		PushEnabled:         settings.GetPushEnabled(),
		PushLikes:           settings.GetPushLikes(),
		PushComments:        settings.GetPushComments(),
		PushFollows:         settings.GetPushFollows(),
		PushMentions:        settings.GetPushMentions(),
		EmailEnabled:        settings.GetEmailEnabled(),
		EmailWeeklyDigest:   settings.GetEmailWeeklyDigest(),
		EmailSecurityAlerts: settings.GetEmailSecurityAlerts(),
		QuietHoursEnabled:   settings.GetQuietHoursEnabled(),
		QuietHoursStart:     settings.GetQuietHoursStart(),
		QuietHoursEnd:       settings.GetQuietHoursEnd(),
		Timezone:            settings.GetTimezone(),
	}
}
