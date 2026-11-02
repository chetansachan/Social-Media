package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// EventType is the normalised verb of a consumed domain event.
type EventType string

const (
	EventUserFollowed   EventType = "user.followed"
	EventUserUnfollowed EventType = "user.unfollowed"
	EventPostLiked      EventType = "post.liked"
	EventPostUnliked    EventType = "post.unliked"
	EventPostCommented  EventType = "post.commented"
	EventCommentReplied EventType = "comment.replied"
	EventUserMentioned  EventType = "user.mentioned"
	EventCreatorPost    EventType = "post.created"
	EventSecurityAlert  EventType = "security.alert"
	EventWelcome        EventType = "user.welcome"
)

// Event is the broker-neutral envelope. Payload stays raw so the transport
// adapter can pass through any producer's event schema.
type Event struct {
	Type    EventType       `json:"type"`
	Payload json.RawMessage `json:"payload"`
	EventID string          `json:"event_id,omitempty"`
}

// DecodePayload unmarshals the event payload into the supplied destination.
func (e Event) DecodePayload(dst any) error {
	if len(e.Payload) == 0 {
		return fmt.Errorf("%w: event payload is empty", ErrInvalidInput)
	}
	if err := json.Unmarshal(e.Payload, dst); err != nil {
		return fmt.Errorf("%w: cannot decode %s payload: %v", ErrInvalidInput, e.Type, err)
	}
	return nil
}

// EventFromValues decodes a Redis Stream message. It supports three layouts:
//
//	{"event": "{\"type\":\"post.liked\",\"payload\":{...}}"}
//	{"type": "post.liked", "payload": "{\"post_id\":\"...\"}"}
//	{"type": "post.liked", "post_id": "...", "actor_id": "..."}
//
// The last form lets producers XADD flat fields without nesting JSON.
func EventFromValues(values map[string]interface{}) (Event, error) {
	if raw, ok := values["event"]; ok {
		data, err := bytesFromValue(raw)
		if err != nil {
			return Event{}, err
		}
		var event Event
		if err := json.Unmarshal(data, &event); err != nil {
			return Event{}, fmt.Errorf("%w: malformed event envelope: %v", ErrInvalidInput, err)
		}
		if event.Type == "" {
			return Event{}, fmt.Errorf("%w: event type is missing", ErrInvalidInput)
		}
		event.Type = NormalizeEventType(string(event.Type))
		return event, nil
	}

	typeValue, ok := values["type"]
	if !ok {
		return Event{}, fmt.Errorf("%w: message has no event envelope", ErrInvalidInput)
	}
	typeString, err := stringFromValue(typeValue)
	if err != nil {
		return Event{}, err
	}

	event := Event{
		Type: NormalizeEventType(typeString),
	}
	if eventID, ok := values["event_id"]; ok {
		event.EventID, err = stringFromValue(eventID)
		if err != nil {
			return Event{}, err
		}
	}

	if payloadValue, ok := values["payload"]; ok {
		event.Payload, err = bytesFromValue(payloadValue)
		if err != nil {
			return Event{}, err
		}
	} else {
		flat := make(map[string]string, len(values))
		for key, value := range values {
			if key == "type" || key == "event_id" {
				continue
			}
			text, err := stringFromValue(value)
			if err != nil {
				return Event{}, err
			}
			flat[key] = text
		}
		encoded, err := json.Marshal(flat)
		if err != nil {
			return Event{}, fmt.Errorf("%w: cannot encode flat payload: %v", ErrInvalidInput, err)
		}
		event.Payload = encoded
	}

	if event.Type == "" {
		return Event{}, fmt.Errorf("%w: event type is missing", ErrInvalidInput)
	}
	return event, nil
}

// NormalizeEventType accepts the many spellings producers use in practice and
// collapses them to the canonical dotted form. Unknown types are preserved
// (normalised) so the event handler can reject them as permanent errors.
func NormalizeEventType(raw string) EventType {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.TrimSuffix(value, "event")
	value = strings.ReplaceAll(value, "-", ".")
	value = strings.ReplaceAll(value, "_", ".")
	value = strings.Trim(value, ".")

	switch value {
	case "user.followed", "userfollowed", "follow.created", "followed", "follow.new":
		return EventUserFollowed
	case "user.unfollowed", "userunfollowed", "unfollow.created", "unfollowed":
		return EventUserUnfollowed
	case "post.liked", "postliked", "like.created", "liked", "post.like":
		return EventPostLiked
	case "post.unliked", "postunliked", "unlike.created", "unliked":
		return EventPostUnliked
	case "post.commented", "postcommented", "comment.created", "commented":
		return EventPostCommented
	case "comment.replied", "commentreplied", "reply.created", "replied":
		return EventCommentReplied
	case "user.mentioned", "usermentioned", "mention.created", "mentioned":
		return EventUserMentioned
	case "post.created", "postcreated", "creator.post", "creatorpost", "post.published":
		return EventCreatorPost
	case "security.alert", "securityalert", "user.security.alert", "account.security.alert":
		return EventSecurityAlert
	case "user.welcome", "userwelcome", "welcome", "welcomemessage":
		return EventWelcome
	default:
		return EventType(value)
	}
}

func stringFromValue(value interface{}) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case []byte:
		return string(typed), nil
	case nil:
		return "", nil
	default:
		return "", fmt.Errorf("%w: unsupported message field type %T", ErrInvalidInput, value)
	}
}

func bytesFromValue(value interface{}) ([]byte, error) {
	switch typed := value.(type) {
	case string:
		return []byte(typed), nil
	case []byte:
		return typed, nil
	default:
		return nil, fmt.Errorf("%w: unsupported message field type %T", ErrInvalidInput, value)
	}
}

// UserFollowedEvent is emitted by follow-service.
type UserFollowedEvent struct {
	FollowerID  string `json:"follower_id"`
	FollowingID string `json:"following_id"`
}

// UserUnfollowedEvent is emitted by follow-service.
type UserUnfollowedEvent struct {
	FollowerID  string `json:"follower_id"`
	FollowingID string `json:"following_id"`
}

// PostReactionEvent covers like/unlike events emitted by post-service. The
// recipient is the post author; some producers call it user_id or
// recipient_id, so all spellings are accepted.
type PostReactionEvent struct {
	PostID           string `json:"post_id"`
	PostAuthorID     string `json:"post_author_id"`
	UserID           string `json:"user_id"`
	RecipientID      string `json:"recipient_id"`
	ActorID          string `json:"actor_id"`
	ActorUsername    string `json:"actor_username"`
	ActorDisplayName string `json:"actor_display_name"`
	ActorAvatarURL   string `json:"actor_avatar_url"`
	PostPreview      string `json:"post_preview"`
}

type PostLikedEvent = PostReactionEvent
type PostUnlikedEvent = PostReactionEvent

// Recipient resolves the post author from the supported field spellings.
func (e PostReactionEvent) Recipient() string {
	return firstNonEmpty(e.PostAuthorID, e.RecipientID, e.UserID)
}

// PostCommentedEvent is emitted when a top-level comment is created.
type PostCommentedEvent struct {
	PostID           string `json:"post_id"`
	CommentID        string `json:"comment_id"`
	PostAuthorID     string `json:"post_author_id"`
	UserID           string `json:"user_id"`
	RecipientID      string `json:"recipient_id"`
	ParentCommentID  string `json:"parent_comment_id"`
	ActorID          string `json:"actor_id"`
	ActorUsername    string `json:"actor_username"`
	ActorDisplayName string `json:"actor_display_name"`
	ActorAvatarURL   string `json:"actor_avatar_url"`
	ContentPreview   string `json:"content_preview"`
}

func (e PostCommentedEvent) Recipient() string {
	return firstNonEmpty(e.PostAuthorID, e.RecipientID, e.UserID)
}

// CommentRepliedEvent is emitted when a comment is posted in reply to another
// comment. The recipient is the parent comment's author.
type CommentRepliedEvent struct {
	PostID           string `json:"post_id"`
	CommentID        string `json:"comment_id"`
	ParentCommentID  string `json:"parent_comment_id"`
	ParentAuthorID   string `json:"parent_author_id"`
	UserID           string `json:"user_id"`
	RecipientID      string `json:"recipient_id"`
	ActorID          string `json:"actor_id"`
	ActorUsername    string `json:"actor_username"`
	ActorDisplayName string `json:"actor_display_name"`
	ActorAvatarURL   string `json:"actor_avatar_url"`
	ContentPreview   string `json:"content_preview"`
}

func (e CommentRepliedEvent) Recipient() string {
	return firstNonEmpty(e.ParentAuthorID, e.RecipientID, e.UserID)
}

// UserMentionedEvent is emitted when an actor mentions a user in a post or
// comment.
type UserMentionedEvent struct {
	MentionedUserID  string `json:"mentioned_user_id"`
	RecipientID      string `json:"recipient_id"`
	UserID           string `json:"user_id"`
	ActorID          string `json:"actor_id"`
	ActorUsername    string `json:"actor_username"`
	ActorDisplayName string `json:"actor_display_name"`
	ActorAvatarURL   string `json:"actor_avatar_url"`
	PostID           string `json:"post_id"`
	CommentID        string `json:"comment_id"`
	ContentPreview   string `json:"content_preview"`
}

func (e UserMentionedEvent) Recipient() string {
	return firstNonEmpty(e.MentionedUserID, e.RecipientID, e.UserID)
}

// CreatorPostEvent notifies a single follower about a creator's new post.
// Fan-out to each follower is the producer's responsibility.
type CreatorPostEvent struct {
	PostID           string `json:"post_id"`
	AuthorID         string `json:"author_id"`
	RecipientID      string `json:"recipient_id"`
	UserID           string `json:"user_id"`
	ActorUsername    string `json:"actor_username"`
	ActorDisplayName string `json:"actor_display_name"`
	ActorAvatarURL   string `json:"actor_avatar_url"`
	PostPreview      string `json:"post_preview"`
}

func (e CreatorPostEvent) Recipient() string {
	return firstNonEmpty(e.RecipientID, e.UserID)
}

// SecurityAlertEvent is a mandatory account notification.
type SecurityAlertEvent struct {
	UserID      string `json:"user_id"`
	RecipientID string `json:"recipient_id"`
	Title       string `json:"title"`
	Message     string `json:"message"`
	ActionURL   string `json:"action_url"`
}

func (e SecurityAlertEvent) Recipient() string {
	return firstNonEmpty(e.RecipientID, e.UserID)
}

// WelcomeMessageEvent is emitted after registration.
type WelcomeMessageEvent struct {
	UserID      string `json:"user_id"`
	RecipientID string `json:"recipient_id"`
	Title       string `json:"title"`
	Message     string `json:"message"`
	ActionURL   string `json:"action_url"`
}

func (e WelcomeMessageEvent) Recipient() string {
	return firstNonEmpty(e.RecipientID, e.UserID)
}

// ScoreFromTimestamp is retained for producers that attach Unix timestamps in
// seconds, millis, micros, or nanos. The notification service persists
// server-side NOW() for ordering, but tests and future consumers can use this
// helper to normalise producer clocks.
func ScoreFromTimestamp(ts int64) int64 {
	if ts <= 0 {
		return time.Now().UnixMicro()
	}
	switch {
	case ts < 1e11:
		return ts * 1e6
	case ts < 1e14:
		return ts * 1e3
	case ts < 1e17:
		return ts
	default:
		return ts / 1e3
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
