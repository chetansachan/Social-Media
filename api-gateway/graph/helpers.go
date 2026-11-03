package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chetansachan/socialflow/api-gateway/auth"
	"github.com/chetansachan/socialflow/api-gateway/graph/dataloader"
	"github.com/chetansachan/socialflow/api-gateway/graph/model"
	socialflowpb "github.com/chetansachan/socialflow/proto"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const (
	defaultFeedPageSize         = 20
	maxFeedPageSize             = 50
	defaultNotificationPageSize = 20
	maxNotificationPageSize     = 50
)

func toModelUser(user *socialflowpb.User) *model.User {
	if user == nil {
		return nil
	}
	return &model.User{
		ID:        user.GetId(),
		Username:  user.GetUsername(),
		Name:      user.GetName(),
		Email:     user.GetEmail(),
		IsPublic:  user.GetIsPublic(),
		CreatedAt: user.GetCreatedAt(),
		UpdatedAt: user.GetUpdatedAt(),
	}
}

func toModelPost(post *socialflowpb.Post) *model.Post {
	if post == nil {
		return nil
	}
	return &model.Post{
		ID:        post.GetId(),
		AuthorID:  post.GetAuthorId(),
		Content:   post.GetContent(),
		MediaURL:  post.GetMediaUrl(),
		IsPublic:  post.GetIsPublic(),
		LikeCount: post.GetLikeCount(),
		CreatedAt: post.GetCreatedAt(),
		UpdatedAt: post.GetUpdatedAt(),
	}
}

func toModelFeedPost(post *socialflowpb.FeedPost) *model.FeedPost {
	if post == nil {
		return nil
	}
	return &model.FeedPost{
		ID:        post.GetId(),
		AuthorID:  post.GetAuthorId(),
		Content:   post.GetContent(),
		MediaURL:  post.GetMediaUrl(),
		IsPublic:  post.GetIsPublic(),
		LikeCount: post.GetLikeCount(),
		CreatedAt: post.GetCreatedAt(),
	}
}

func toModelNotificationActor(actor *socialflowpb.NotificationActor) *model.NotificationActor {
	if actor == nil {
		return nil
	}
	return &model.NotificationActor{
		ID:          actor.GetId(),
		Username:    actor.GetUsername(),
		DisplayName: actor.GetDisplayName(),
		AvatarURL:   actor.GetAvatarUrl(),
	}
}

func toModelNotification(item *socialflowpb.NotificationItem) *model.NotificationItem {
	if item == nil {
		return nil
	}

	actors := make([]*model.NotificationActor, 0, len(item.GetActors()))
	for _, actor := range item.GetActors() {
		actors = append(actors, toModelNotificationActor(actor))
	}

	return &model.NotificationItem{
		ID:               item.GetId(),
		Type:             notificationTypeFromProto(item.GetType()),
		EntityID:         item.GetEntityId(),
		EntityType:       item.GetEntityType(),
		Actors:           actors,
		TotalActorsCount: item.GetTotalActorsCount(),
		Title:            item.GetTitle(),
		ContentPreview:   optionalString(item.GetContentPreview()),
		ActionURL:        item.GetActionUrl(),
		IsRead:           item.GetIsRead(),
		CreatedAt:        item.GetCreatedAt(),
	}
}

func toModelNotificationSettings(settings *socialflowpb.NotificationSettings) *model.NotificationSettings {
	if settings == nil {
		return &model.NotificationSettings{}
	}
	return &model.NotificationSettings{
		InappLikes:          settings.GetInappLikes(),
		InappComments:       settings.GetInappComments(),
		InappFollows:        settings.GetInappFollows(),
		InappMentions:       settings.GetInappMentions(),
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

func notificationTypeFromProto(value socialflowpb.NotificationType) model.NotificationType {
	switch value {
	case socialflowpb.NotificationType_NOTIFICATION_TYPE_FOLLOW:
		return model.NotificationTypeFollow
	case socialflowpb.NotificationType_NOTIFICATION_TYPE_POST_LIKE:
		return model.NotificationTypePostLike
	case socialflowpb.NotificationType_NOTIFICATION_TYPE_COMMENT:
		return model.NotificationTypeComment
	case socialflowpb.NotificationType_NOTIFICATION_TYPE_COMMENT_REPLY:
		return model.NotificationTypeCommentReply
	case socialflowpb.NotificationType_NOTIFICATION_TYPE_MENTION:
		return model.NotificationTypeMention
	case socialflowpb.NotificationType_NOTIFICATION_TYPE_CREATOR_POST:
		return model.NotificationTypeCreatorPost
	case socialflowpb.NotificationType_NOTIFICATION_TYPE_SYSTEM_ALERT:
		return model.NotificationTypeSystemAlert
	default:
		// New producer categories should not break an inbox page; surface them
		// as a system alert until the gateway schema learns the new value.
		return model.NotificationTypeSystemAlert
	}
}

func notificationTypeFromString(value string) model.NotificationType {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimPrefix(normalized, "notification_type_")
	normalized = strings.ReplaceAll(normalized, "-", "_")

	switch normalized {
	case "follow":
		return model.NotificationTypeFollow
	case "post_like", "like":
		return model.NotificationTypePostLike
	case "comment":
		return model.NotificationTypeComment
	case "comment_reply", "reply":
		return model.NotificationTypeCommentReply
	case "mention":
		return model.NotificationTypeMention
	case "creator_post":
		return model.NotificationTypeCreatorPost
	default:
		return model.NotificationTypeSystemAlert
	}
}

func notificationTypeToProto(value *model.NotificationType) socialflowpb.NotificationType {
	if value == nil {
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED
	}
	switch *value {
	case model.NotificationTypeFollow:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_FOLLOW
	case model.NotificationTypePostLike:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_POST_LIKE
	case model.NotificationTypeComment:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_COMMENT
	case model.NotificationTypeCommentReply:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_COMMENT_REPLY
	case model.NotificationTypeMention:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_MENTION
	case model.NotificationTypeCreatorPost:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_CREATOR_POST
	case model.NotificationTypeSystemAlert:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_SYSTEM_ALERT
	default:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED
	}
}

func notificationEntityType(value *model.NotificationType) string {
	if value == nil {
		return "system"
	}
	switch *value {
	case model.NotificationTypeFollow:
		return "user"
	case model.NotificationTypeComment, model.NotificationTypeCommentReply, model.NotificationTypeMention:
		return "comment"
	case model.NotificationTypeSystemAlert:
		return "system"
	default:
		return "post"
	}
}

func toProtoSettings(settings model.NotificationSettings) *socialflowpb.NotificationSettings {
	return &socialflowpb.NotificationSettings{
		InappLikes:          settings.InappLikes,
		InappComments:       settings.InappComments,
		InappFollows:        settings.InappFollows,
		InappMentions:       settings.InappMentions,
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

// applySettings overlays only the fields the client supplied, preserving every
// stored preference the request did not mention.
func applySettings(current *model.NotificationSettings, input model.UpdateNotificationSettingsInput) model.NotificationSettings {
	next := *current
	if input.InappLikes != nil {
		next.InappLikes = *input.InappLikes
	}
	if input.InappComments != nil {
		next.InappComments = *input.InappComments
	}
	if input.InappFollows != nil {
		next.InappFollows = *input.InappFollows
	}
	if input.InappMentions != nil {
		next.InappMentions = *input.InappMentions
	}
	if input.PushEnabled != nil {
		next.PushEnabled = *input.PushEnabled
	}
	if input.PushLikes != nil {
		next.PushLikes = *input.PushLikes
	}
	if input.PushComments != nil {
		next.PushComments = *input.PushComments
	}
	if input.PushFollows != nil {
		next.PushFollows = *input.PushFollows
	}
	if input.PushMentions != nil {
		next.PushMentions = *input.PushMentions
	}
	if input.EmailEnabled != nil {
		next.EmailEnabled = *input.EmailEnabled
	}
	if input.EmailWeeklyDigest != nil {
		next.EmailWeeklyDigest = *input.EmailWeeklyDigest
	}
	if input.EmailSecurityAlerts != nil {
		next.EmailSecurityAlerts = *input.EmailSecurityAlerts
	}
	if input.QuietHoursEnabled != nil {
		next.QuietHoursEnabled = *input.QuietHoursEnabled
	}
	if input.QuietHoursStart != nil {
		next.QuietHoursStart = *input.QuietHoursStart
	}
	if input.QuietHoursEnd != nil {
		next.QuietHoursEnd = *input.QuietHoursEnd
	}
	if input.Timezone != nil {
		next.Timezone = *input.Timezone
	}
	return next
}

// requireViewer returns the authenticated caller and a GraphQL-facing error
// when the request is anonymous.
func requireViewer(ctx context.Context) (auth.Principal, error) {
	return auth.Require(ctx)
}

// requireSelf rejects requests that try to act on behalf of another user.
// Authorization is decided from the verified token, never from client input.
func requireSelf(principal auth.Principal, requestedUserID string) error {
	if strings.TrimSpace(requestedUserID) == "" {
		return nil
	}
	if requestedUserID != principal.UserID {
		return &gqlerror.Error{
			Message:    "cannot act on behalf of another user",
			Extensions: map[string]any{"code": "FORBIDDEN"},
		}
	}
	return nil
}

func clampPageSize(requested *int32, fallback, maximum int32) int32 {
	if requested == nil || *requested <= 0 {
		return fallback
	}
	if *requested > maximum {
		return maximum
	}
	return *requested
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// loadAuthor resolves a post author through the request-scoped user loader so
// a feed page issues one batch lookup instead of one RPC per post.
func loadAuthor(ctx context.Context, authorID string) (*model.User, error) {
	loaders := dataloader.FromContext(ctx)
	if loaders == nil || loaders.UserByID == nil {
		return nil, fmt.Errorf("dataloader middleware is not configured")
	}
	user, err := loaders.RequireUser(ctx, authorID)
	if err != nil {
		return nil, err
	}
	return toModelUser(user), nil
}

// loadLikeState returns the viewer's like state for one post. Anonymous
// callers always see false rather than an authorization error: like state is
// public metadata, not private data.
func loadLikeState(ctx context.Context, postID string) (bool, error) {
	principal, ok := auth.FromContext(ctx)
	if !ok || strings.TrimSpace(principal.UserID) == "" {
		return false, nil
	}

	loaders := dataloader.FromContext(ctx)
	if loaders == nil || loaders.LikeState == nil {
		return false, fmt.Errorf("dataloader middleware is not configured")
	}

	liked, err := loaders.LikeState.Load(ctx, dataloader.LikeKey{
		ViewerID: principal.UserID,
		PostID:   postID,
	})()
	if err != nil {
		return false, err
	}
	return liked, nil
}

// requirePostOwner rejects a mutation aimed at somebody else's post. The
// owner is read from post-service, never from client input.
func requirePostOwner(principal auth.Principal, authorID string) error {
	if authorID == principal.UserID {
		return nil
	}
	return &gqlerror.Error{
		Message:    "cannot modify another user's post",
		Extensions: map[string]any{"code": "FORBIDDEN"},
	}
}

// requirePostOwnership loads the post first so delete and archive cannot be
// used to probe or mutate another user's content.
func (r *Resolver) requirePostOwnership(ctx context.Context, principal auth.Principal, postID string) error {
	res, err := r.PostClient.GetPostById(ctx, &socialflowpb.PostByIdRequest{Id: postID})
	if err != nil {
		return err
	}
	return requirePostOwner(principal, res.GetPost().GetAuthorId())
}

// realtimeChannel is the Redis Pub/Sub channel notification-service publishes
// one user's frames to.
func realtimeChannel(userID string) string {
	return "notify:" + strings.TrimSpace(userID)
}

// realtimeFrame mirrors the WebSocket envelope published by notification-service.
type realtimeFrame struct {
	Event string                   `json:"event"`
	Data  realtimeNotificationData `json:"data"`
}

type realtimeNotificationData struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	Title            string          `json:"title"`
	ContentPreview   string          `json:"content_preview"`
	ActionURL        string          `json:"action_url"`
	Actors           []realtimeActor `json:"actors"`
	TotalActorsCount int32           `json:"total_actors_count"`
	UnreadCount      int32           `json:"unread_count"`
	CreatedAt        string          `json:"created_at"`
}

type realtimeActor struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

// notificationItemFromFrame converts a realtime frame into the GraphQL type.
// The frame intentionally omits entity id, entity type, and read state, so they
// are derived here rather than forcing the dispatcher to send another query.
func notificationItemFromFrame(payload []byte) (*model.NotificationItem, error) {
	var frame realtimeFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		return nil, fmt.Errorf("decode realtime notification: %w", err)
	}
	if strings.TrimSpace(frame.Data.ID) == "" {
		return nil, fmt.Errorf("realtime notification is missing an id")
	}

	itemType := notificationTypeFromString(frame.Data.Type)
	actors := make([]*model.NotificationActor, 0, len(frame.Data.Actors))
	for _, actor := range frame.Data.Actors {
		actors = append(actors, &model.NotificationActor{
			ID:          actor.ID,
			Username:    actor.Username,
			DisplayName: actor.DisplayName,
			AvatarURL:   actor.AvatarURL,
		})
	}

	// A follow notification points at the follower's profile, so the single
	// actor is also the entity. Other categories reference content the compact
	// frame does not name; clients refetch the inbox row before navigating.
	entityID := ""
	if itemType == model.NotificationTypeFollow && len(actors) == 1 {
		entityID = actors[0].ID
	}

	return &model.NotificationItem{
		ID:               frame.Data.ID,
		Type:             itemType,
		EntityID:         entityID,
		EntityType:       notificationEntityType(&itemType),
		Actors:           actors,
		TotalActorsCount: frame.Data.TotalActorsCount,
		Title:            frame.Data.Title,
		ContentPreview:   optionalString(frame.Data.ContentPreview),
		ActionURL:        frame.Data.ActionURL,
		IsRead:           false,
		CreatedAt:        frame.Data.CreatedAt,
	}, nil
}
