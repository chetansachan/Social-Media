package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// NotificationType is the persisted category of a notification. The string
// values map one-to-one with the PostgreSQL notification_type enum.
type NotificationType string

const (
	TypeFollow       NotificationType = "follow"
	TypePostLike     NotificationType = "post_like"
	TypeComment      NotificationType = "comment"
	TypeCommentReply NotificationType = "comment_reply"
	TypeMention      NotificationType = "mention"
	TypeCreatorPost  NotificationType = "creator_post"
	TypeSystemAlert  NotificationType = "system_alert"
)

// ParseNotificationType accepts database values, proto enum names, and loose
// client input ("post_like", "POST_LIKE", "NOTIFICATION_TYPE_POST_LIKE").
func ParseNotificationType(value string) (NotificationType, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimPrefix(normalized, "notification_type_")
	normalized = strings.ReplaceAll(normalized, "-", "_")

	switch normalized {
	case "follow", "follower":
		return TypeFollow, nil
	case "post_like", "like", "likes":
		return TypePostLike, nil
	case "comment", "post_comment":
		return TypeComment, nil
	case "comment_reply", "reply":
		return TypeCommentReply, nil
	case "mention", "user_mention":
		return TypeMention, nil
	case "creator_post", "new_post":
		return TypeCreatorPost, nil
	case "system_alert", "security_alert", "system":
		return TypeSystemAlert, nil
	default:
		return "", fmt.Errorf("%w: unknown notification type %q", ErrInvalidInput, value)
	}
}

func (t NotificationType) IsValid() bool {
	switch t {
	case TypeFollow, TypePostLike, TypeComment, TypeCommentReply, TypeMention, TypeCreatorPost, TypeSystemAlert:
		return true
	default:
		return false
	}
}

// Category maps a notification type to the preference toggle that controls it.
func (t NotificationType) Category() string {
	switch t {
	case TypeFollow, TypeCreatorPost:
		return "follows"
	case TypePostLike:
		return "likes"
	case TypeComment, TypeCommentReply:
		return "comments"
	case TypeMention:
		return "mentions"
	case TypeSystemAlert:
		return "system"
	default:
		return ""
	}
}

// Aggregatable reports whether repeated events on the same entity should be
// collapsed into one inbox row. Likes are the high-volume case; comments and
// mentions are intentionally kept individual because each one carries distinct
// content.
func (t NotificationType) Aggregatable() bool {
	return t == TypePostLike
}

// EntityTypeFor returns the database entity_type for a notification type.
func (t NotificationType) EntityTypeFor() string {
	switch t {
	case TypeFollow:
		return "user"
	case TypeComment, TypeCommentReply, TypeMention:
		return "comment"
	case TypeSystemAlert:
		return "system"
	default:
		return "post"
	}
}

// Actor is a lightweight projection of the user who caused a notification.
type Actor struct {
	ID          string `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Email       string `json:"-"`
}

// Name returns the best available human-readable name.
func (a Actor) Name() string {
	if strings.TrimSpace(a.DisplayName) != "" {
		return a.DisplayName
	}
	if strings.TrimSpace(a.Username) != "" {
		return a.Username
	}
	if a.ID != "" {
		if len(a.ID) > 8 {
			return "User " + a.ID[:8]
		}
		return "User " + a.ID
	}
	return "Someone"
}

// Notification is a persisted inbox row.
type Notification struct {
	ID               uuid.UUID        `json:"id"`
	RecipientID      uuid.UUID        `json:"recipient_id"`
	Type             NotificationType `json:"type"`
	EntityID         string           `json:"entity_id"`
	EntityType       string           `json:"entity_type"`
	ActorIDs         []string         `json:"actor_ids"`
	Actors           []Actor          `json:"actors,omitempty"`
	TotalActorsCount int              `json:"total_actors_count"`
	Title            string           `json:"title"`
	ContentPreview   string           `json:"content_preview"`
	ActionURL        string           `json:"action_url"`
	IsRead           bool             `json:"is_read"`
	ReadAt           *time.Time       `json:"read_at,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	DeletedAt        *time.Time       `json:"deleted_at,omitempty"`
}

// CreateNotificationInput carries the fields required to insert a new inbox
// row. ID and timestamps are assigned by PostgreSQL.
type CreateNotificationInput struct {
	RecipientID      uuid.UUID
	Type             NotificationType
	EntityID         string
	EntityType       string
	ActorIDs         []string
	TotalActorsCount int
	Title            string
	ContentPreview   string
	ActionURL        string
}

// NotificationPage is one cursor-paginated slice of the inbox.
type NotificationPage struct {
	Items       []Notification
	NextCursor  string
	HasMore     bool
	UnreadCount int32
}

// NotificationRequest is the transport-neutral command produced by the event
// handler and consumed by the notification service. Keeping this separate from
// broker payloads means the service can also be invoked from tests or internal
// admin tooling without constructing a JSON envelope.
type NotificationRequest struct {
	RecipientID uuid.UUID
	ActorID     *uuid.UUID

	ActorUsername    string
	ActorDisplayName string
	ActorAvatarURL   string

	Type       NotificationType
	EntityID   string
	EntityType string

	ActionURL      string
	ContentPreview string

	// TitleOverride and BodyOverride are used by system/security alerts.
	TitleOverride string
	BodyOverride  string

	// BypassPolicy is set for mandatory security alerts.
	BypassPolicy bool

	// EventID, when supplied by the producer, makes idempotency global rather
	// than keyed on the actor/entity pair.
	EventID string
}

// DedupKey builds the Redis idempotency key for this request. The recipient is
// part of the key because one actor can legitimately trigger the same verb on
// the same entity for different recipients (for example a follow).
func (r NotificationRequest) DedupKey() string {
	if r.EventID != "" {
		return "dedup:event:" + r.EventID
	}
	actor := "system"
	if r.ActorID != nil {
		actor = r.ActorID.String()
	}
	return fmt.Sprintf("dedup:%s:%s:%s:%s", r.RecipientID, actor, r.Type, r.EntityID)
}

// Validate performs cheap structural checks before any persistence happens.
func (r NotificationRequest) Validate() error {
	if r.RecipientID == uuid.Nil {
		return fmt.Errorf("%w: recipient_id is required", ErrInvalidInput)
	}
	if !r.Type.IsValid() {
		return fmt.Errorf("%w: notification type is invalid", ErrInvalidInput)
	}
	if strings.TrimSpace(r.EntityID) == "" {
		// System alerts are not attached to a domain entity.
		if r.Type != TypeSystemAlert {
			return fmt.Errorf("%w: entity_id is required", ErrInvalidInput)
		}
	}
	if r.ActorID != nil && *r.ActorID == r.RecipientID {
		return fmt.Errorf("%w: actor and recipient are the same user", ErrInvalidInput)
	}
	return nil
}

// IsSelfAction reports whether an actor is acting on their own content.
func IsSelfAction(actorID *uuid.UUID, recipientID uuid.UUID) bool {
	return actorID != nil && *actorID == recipientID
}

// RenderTitle produces the human-readable headline stored on the row and
// re-rendered after actor hydration.
func RenderTitle(notificationType NotificationType, actors []Actor, total int) string {
	if total < len(actors) {
		total = len(actors)
	}
	if total < 1 {
		total = 1
	}

	subject := summarizeActors(actors, total)

	switch notificationType {
	case TypeFollow:
		return subject + " started following you"
	case TypePostLike:
		return subject + " liked your post"
	case TypeComment:
		return subject + " commented on your post"
	case TypeCommentReply:
		return subject + " replied to your comment"
	case TypeMention:
		return subject + " mentioned you"
	case TypeCreatorPost:
		return subject + " shared a new post"
	default:
		return subject
	}
}

// summarizeActors renders "Alice", "Alice and Bob", "Alice, Bob, and 5 others".
func summarizeActors(actors []Actor, total int) string {
	names := make([]string, 0, len(actors))
	for _, actor := range actors {
		if len(names) == 3 {
			break
		}
		names = append(names, actor.Name())
	}
	if len(names) == 0 {
		return "Someone"
	}

	switch {
	case total <= 1:
		return names[0]
	case total == 2 && len(names) >= 2:
		return names[0] + " and " + names[1]
	case total <= len(names) && total <= 3:
		return strings.Join(names[:total-1], ", ") + ", and " + names[total-1]
	default:
		remaining := total - len(names)
		word := "others"
		if remaining == 1 {
			word = "other"
		}
		if len(names) == 1 {
			return fmt.Sprintf("%s and %d %s", names[0], remaining, word)
		}
		return strings.Join(names, ", ") + fmt.Sprintf(", and %d %s", remaining, word)
	}
}

// RenderPreview trims the preview to the column width and supplies a sensible
// fallback for types that normally carry content.
func RenderPreview(notificationType NotificationType, preview string) string {
	preview = strings.TrimSpace(preview)
	if preview == "" {
		switch notificationType {
		case TypeFollow:
			return "You have a new follower"
		case TypePostLike:
			return "Your post is getting attention"
		case TypeComment, TypeCommentReply:
			return "There is a new comment on your content"
		case TypeMention:
			return "You were mentioned in a conversation"
		case TypeCreatorPost:
			return "A creator you follow published something new"
		default:
			return ""
		}
	}
	runes := []rune(preview)
	if len(runes) > 255 {
		return string(runes[:252]) + "..."
	}
	return preview
}

// DefaultActionURL builds a deep link when the producer did not supply one.
func DefaultActionURL(notificationType NotificationType, entityID, actorID string) string {
	switch notificationType {
	case TypeFollow:
		if actorID != "" {
			return "socialflow://users/" + actorID
		}
		return "socialflow://notifications"
	case TypeSystemAlert:
		return "socialflow://settings/security"
	default:
		if entityID != "" {
			return "socialflow://posts/" + entityID
		}
		return "socialflow://notifications"
	}
}
