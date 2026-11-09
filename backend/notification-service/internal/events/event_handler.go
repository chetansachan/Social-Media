package events

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/chetansachan/socialflow/notification-service/internal/services"
	"github.com/google/uuid"
)

// EventHandler translates producer-specific event schemas into the
// NotificationRequest understood by the service layer. It is the only place
// that knows both worlds.
type EventHandler struct {
	service services.NotificationService
	logger  *log.Logger
}

func NewEventHandler(service services.NotificationService, logger *log.Logger) *EventHandler {
	if logger == nil {
		logger = log.Default()
	}
	return &EventHandler{service: service, logger: logger}
}

func (h *EventHandler) HandleEvent(ctx context.Context, event models.Event) error {
	switch event.Type {
	case models.EventUserFollowed:
		return h.handleFollowed(ctx, event)
	case models.EventUserUnfollowed:
		// Unfollowing does not retract a historical "started following you"
		// notification; that would rewrite history. Nothing to do.
		return nil
	case models.EventPostLiked:
		return h.handleLiked(ctx, event)
	case models.EventPostUnliked:
		return h.handleUnliked(ctx, event)
	case models.EventPostCommented:
		return h.handleCommented(ctx, event)
	case models.EventCommentReplied:
		return h.handleReplied(ctx, event)
	case models.EventUserMentioned:
		return h.handleMentioned(ctx, event)
	case models.EventCreatorPost:
		return h.handleCreatorPost(ctx, event)
	case models.EventSecurityAlert:
		return h.handleSecurityAlert(ctx, event)
	case models.EventWelcome:
		return h.handleWelcome(ctx, event)
	default:
		return fmt.Errorf("%w: unsupported event type %q", models.ErrPermanentEvent, event.Type)
	}
}

func (h *EventHandler) handleFollowed(ctx context.Context, event models.Event) error {
	var payload models.UserFollowedEvent
	if err := decode(event, &payload); err != nil {
		return err
	}

	actorID, err := parseRequiredID(payload.FollowerID, "follower_id")
	if err != nil {
		return err
	}
	recipientID, err := parseRequiredID(payload.FollowingID, "following_id")
	if err != nil {
		return err
	}

	return h.service.ProcessEvent(ctx, &models.NotificationRequest{
		RecipientID: recipientID,
		ActorID:     &actorID,
		Type:        models.TypeFollow,
		EntityID:    actorID.String(),
		EntityType:  "user",
		ActionURL:   "socialflow://users/" + actorID.String(),
		EventID:     event.EventID,
	})
}

func (h *EventHandler) handleLiked(ctx context.Context, event models.Event) error {
	var payload models.PostLikedEvent
	if err := decode(event, &payload); err != nil {
		return err
	}

	recipientID, actorID, err := reactionIDs(payload)
	if err != nil {
		return err
	}

	return h.service.ProcessEvent(ctx, &models.NotificationRequest{
		RecipientID:      recipientID,
		ActorID:          &actorID,
		ActorUsername:    payload.ActorUsername,
		ActorDisplayName: payload.ActorDisplayName,
		ActorAvatarURL:   payload.ActorAvatarURL,
		Type:             models.TypePostLike,
		EntityID:         strings.TrimSpace(payload.PostID),
		EntityType:       "post",
		ActionURL:        postURL(payload.PostID),
		ContentPreview:   payload.PostPreview,
		EventID:          event.EventID,
	})
}

func (h *EventHandler) handleUnliked(ctx context.Context, event models.Event) error {
	var payload models.PostUnlikedEvent
	if err := decode(event, &payload); err != nil {
		return err
	}

	recipientID, actorID, err := reactionIDs(payload)
	if err != nil {
		return err
	}

	return h.service.RevokeAggregatedActor(ctx, &models.NotificationRequest{
		RecipientID:      recipientID,
		ActorID:          &actorID,
		ActorUsername:    payload.ActorUsername,
		ActorDisplayName: payload.ActorDisplayName,
		ActorAvatarURL:   payload.ActorAvatarURL,
		Type:             models.TypePostLike,
		EntityID:         strings.TrimSpace(payload.PostID),
		EntityType:       "post",
		ActionURL:        postURL(payload.PostID),
		ContentPreview:   payload.PostPreview,
		EventID:          event.EventID,
	})
}

func (h *EventHandler) handleCommented(ctx context.Context, event models.Event) error {
	var payload models.PostCommentedEvent
	if err := decode(event, &payload); err != nil {
		return err
	}

	recipientID, err := parseRequiredID(payload.Recipient(), "post_author_id")
	if err != nil {
		return err
	}
	actorID, err := parseRequiredID(payload.ActorID, "actor_id")
	if err != nil {
		return err
	}
	if strings.TrimSpace(payload.PostID) == "" {
		return permanent("post_id is required")
	}
	if strings.TrimSpace(payload.CommentID) == "" {
		return permanent("comment_id is required")
	}

	return h.service.ProcessEvent(ctx, &models.NotificationRequest{
		RecipientID:      recipientID,
		ActorID:          &actorID,
		ActorUsername:    payload.ActorUsername,
		ActorDisplayName: payload.ActorDisplayName,
		ActorAvatarURL:   payload.ActorAvatarURL,
		Type:             models.TypeComment,
		EntityID:         payload.CommentID,
		EntityType:       "comment",
		ActionURL:        commentURL(payload.PostID, payload.CommentID),
		ContentPreview:   payload.ContentPreview,
		EventID:          event.EventID,
	})
}

func (h *EventHandler) handleReplied(ctx context.Context, event models.Event) error {
	var payload models.CommentRepliedEvent
	if err := decode(event, &payload); err != nil {
		return err
	}

	recipientID, err := parseRequiredID(payload.Recipient(), "parent_author_id")
	if err != nil {
		return err
	}
	actorID, err := parseRequiredID(payload.ActorID, "actor_id")
	if err != nil {
		return err
	}
	if strings.TrimSpace(payload.CommentID) == "" {
		return permanent("comment_id is required")
	}

	return h.service.ProcessEvent(ctx, &models.NotificationRequest{
		RecipientID:      recipientID,
		ActorID:          &actorID,
		ActorUsername:    payload.ActorUsername,
		ActorDisplayName: payload.ActorDisplayName,
		ActorAvatarURL:   payload.ActorAvatarURL,
		Type:             models.TypeCommentReply,
		EntityID:         payload.CommentID,
		EntityType:       "comment",
		ActionURL:        commentURL(payload.PostID, payload.CommentID),
		ContentPreview:   payload.ContentPreview,
		EventID:          event.EventID,
	})
}

func (h *EventHandler) handleMentioned(ctx context.Context, event models.Event) error {
	var payload models.UserMentionedEvent
	if err := decode(event, &payload); err != nil {
		return err
	}

	recipientID, err := parseRequiredID(payload.Recipient(), "mentioned_user_id")
	if err != nil {
		return err
	}
	actorID, err := parseRequiredID(payload.ActorID, "actor_id")
	if err != nil {
		return err
	}

	entityID := strings.TrimSpace(payload.CommentID)
	entityType := "comment"
	actionURL := commentURL(payload.PostID, payload.CommentID)
	if entityID == "" {
		entityID = strings.TrimSpace(payload.PostID)
		entityType = "post"
		actionURL = postURL(payload.PostID)
	}
	if entityID == "" {
		return permanent("post_id or comment_id is required")
	}

	return h.service.ProcessEvent(ctx, &models.NotificationRequest{
		RecipientID:      recipientID,
		ActorID:          &actorID,
		ActorUsername:    payload.ActorUsername,
		ActorDisplayName: payload.ActorDisplayName,
		ActorAvatarURL:   payload.ActorAvatarURL,
		Type:             models.TypeMention,
		EntityID:         entityID,
		EntityType:       entityType,
		ActionURL:        actionURL,
		ContentPreview:   payload.ContentPreview,
		EventID:          event.EventID,
	})
}

func (h *EventHandler) handleCreatorPost(ctx context.Context, event models.Event) error {
	var payload models.CreatorPostEvent
	if err := decode(event, &payload); err != nil {
		return err
	}

	recipientID, err := parseRequiredID(payload.Recipient(), "recipient_id")
	if err != nil {
		return err
	}
	actorID, err := parseRequiredID(payload.AuthorID, "author_id")
	if err != nil {
		return err
	}
	if strings.TrimSpace(payload.PostID) == "" {
		return permanent("post_id is required")
	}

	return h.service.ProcessEvent(ctx, &models.NotificationRequest{
		RecipientID:      recipientID,
		ActorID:          &actorID,
		ActorUsername:    payload.ActorUsername,
		ActorDisplayName: payload.ActorDisplayName,
		ActorAvatarURL:   payload.ActorAvatarURL,
		Type:             models.TypeCreatorPost,
		EntityID:         payload.PostID,
		EntityType:       "post",
		ActionURL:        postURL(payload.PostID),
		ContentPreview:   payload.PostPreview,
		EventID:          event.EventID,
	})
}

func (h *EventHandler) handleSecurityAlert(ctx context.Context, event models.Event) error {
	var payload models.SecurityAlertEvent
	if err := decode(event, &payload); err != nil {
		return err
	}

	recipientID, err := parseRequiredID(payload.Recipient(), "user_id")
	if err != nil {
		return err
	}

	title := strings.TrimSpace(payload.Title)
	if title == "" {
		title = "Security alert"
	}
	actionURL := strings.TrimSpace(payload.ActionURL)
	if actionURL == "" {
		actionURL = "socialflow://settings/security"
	}

	return h.service.ProcessEvent(ctx, &models.NotificationRequest{
		RecipientID:   recipientID,
		Type:          models.TypeSystemAlert,
		EntityID:      "system",
		EntityType:    "system",
		ActionURL:     actionURL,
		TitleOverride: title,
		BodyOverride:  payload.Message,
		BypassPolicy:  true,
		EventID:       event.EventID,
	})
}

func (h *EventHandler) handleWelcome(ctx context.Context, event models.Event) error {
	var payload models.WelcomeMessageEvent
	if err := decode(event, &payload); err != nil {
		return err
	}

	recipientID, err := parseRequiredID(payload.Recipient(), "user_id")
	if err != nil {
		return err
	}

	title := strings.TrimSpace(payload.Title)
	if title == "" {
		title = "Welcome to SocialFlow"
	}
	message := strings.TrimSpace(payload.Message)
	if message == "" {
		message = "Your account is ready. Follow a few people to start building your feed."
	}
	actionURL := strings.TrimSpace(payload.ActionURL)
	if actionURL == "" {
		actionURL = "socialflow://explore"
	}

	return h.service.ProcessEvent(ctx, &models.NotificationRequest{
		RecipientID:   recipientID,
		Type:          models.TypeSystemAlert,
		EntityID:      "system",
		EntityType:    "system",
		ActionURL:     actionURL,
		TitleOverride: title,
		BodyOverride:  message,
		BypassPolicy:  true,
		EventID:       event.EventID,
	})
}

func reactionIDs(payload models.PostReactionEvent) (uuid.UUID, uuid.UUID, error) {
	if strings.TrimSpace(payload.PostID) == "" {
		return uuid.Nil, uuid.Nil, permanent("post_id is required")
	}
	recipientID, err := parseRequiredID(payload.Recipient(), "post_author_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	actorID, err := parseRequiredID(payload.ActorID, "actor_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return recipientID, actorID, nil
}

func decode(event models.Event, destination any) error {
	if err := event.DecodePayload(destination); err != nil {
		return fmt.Errorf("%w: %v", models.ErrPermanentEvent, err)
	}
	return nil
}

func parseRequiredID(raw, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, permanent(field + " is missing or invalid")
	}
	return id, nil
}

func permanent(message string) error {
	return fmt.Errorf("%w: %s", models.ErrPermanentEvent, message)
}

func postURL(postID string) string {
	if strings.TrimSpace(postID) == "" {
		return "socialflow://notifications"
	}
	return "socialflow://posts/" + postID
}

func commentURL(postID, commentID string) string {
	if strings.TrimSpace(postID) == "" {
		return postURL("")
	}
	if strings.TrimSpace(commentID) == "" {
		return postURL(postID)
	}
	return postURL(postID) + "#comment-" + commentID
}
