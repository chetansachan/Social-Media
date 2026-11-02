package events

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/chetansachan/socialflow/notification-service/internal/services"
	"github.com/google/uuid"
)

// stubService embeds the interface so tests only need to observe the two write
// paths they exercise.
type stubService struct {
	services.NotificationService

	processed []models.NotificationRequest
	revoked   []models.NotificationRequest
	err       error
}

func (s *stubService) ProcessEvent(ctx context.Context, request *models.NotificationRequest) error {
	if s.err != nil {
		return s.err
	}
	s.processed = append(s.processed, *request)
	return nil
}

func (s *stubService) RevokeAggregatedActor(ctx context.Context, request *models.NotificationRequest) error {
	if s.err != nil {
		return s.err
	}
	s.revoked = append(s.revoked, *request)
	return nil
}

func newEvent(t *testing.T, eventType models.EventType, payload any, eventID string) models.Event {
	t.Helper()

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return models.Event{Type: eventType, Payload: encoded, EventID: eventID}
}

func TestHandleFollowed(t *testing.T) {
	t.Parallel()

	service := &stubService{}
	handler := NewEventHandler(service, nil)

	follower := uuid.New()
	following := uuid.New()
	event := newEvent(t, models.EventUserFollowed, models.UserFollowedEvent{
		FollowerID:  follower.String(),
		FollowingID: following.String(),
	}, "evt-1")

	if err := handler.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if len(service.processed) != 1 {
		t.Fatalf("processed = %d, want 1", len(service.processed))
	}

	request := service.processed[0]
	if request.Type != models.TypeFollow {
		t.Errorf("type = %q", request.Type)
	}
	if request.RecipientID != following || request.ActorID == nil || *request.ActorID != follower {
		t.Errorf("recipient/actor mismatch: %+v", request)
	}
	if request.EventID != "evt-1" {
		t.Errorf("event_id = %q", request.EventID)
	}
	if request.ActionURL != "socialflow://users/"+follower.String() {
		t.Errorf("action URL = %q", request.ActionURL)
	}
}

func TestHandleLiked(t *testing.T) {
	t.Parallel()

	service := &stubService{}
	handler := NewEventHandler(service, nil)

	postAuthor := uuid.New()
	actor := uuid.New()
	event := newEvent(t, models.EventPostLiked, models.PostLikedEvent{
		PostID:       "post-1",
		PostAuthorID: postAuthor.String(),
		ActorID:      actor.String(),
		PostPreview:  "hello world",
	}, "")

	if err := handler.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	request := service.processed[0]
	if request.Type != models.TypePostLike {
		t.Errorf("type = %q", request.Type)
	}
	if request.EntityID != "post-1" || request.EntityType != "post" {
		t.Errorf("entity = %q/%q", request.EntityID, request.EntityType)
	}
	if request.ContentPreview != "hello world" {
		t.Errorf("preview = %q", request.ContentPreview)
	}
}

func TestHandleUnlikedRevokes(t *testing.T) {
	t.Parallel()

	service := &stubService{}
	handler := NewEventHandler(service, nil)

	event := newEvent(t, models.EventPostUnliked, models.PostUnlikedEvent{
		PostID:       "post-1",
		PostAuthorID: uuid.NewString(),
		ActorID:      uuid.NewString(),
	}, "")

	if err := handler.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if len(service.revoked) != 1 {
		t.Fatalf("revoked = %d, want 1", len(service.revoked))
	}
	if len(service.processed) != 0 {
		t.Errorf("unlike should not create a notification, got %d", len(service.processed))
	}
}

func TestHandleCommented(t *testing.T) {
	t.Parallel()

	service := &stubService{}
	handler := NewEventHandler(service, nil)

	event := newEvent(t, models.EventPostCommented, models.PostCommentedEvent{
		PostID:       "post-1",
		CommentID:    "comment-1",
		PostAuthorID: uuid.NewString(),
		ActorID:      uuid.NewString(),
	}, "")

	if err := handler.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	request := service.processed[0]
	if request.Type != models.TypeComment {
		t.Errorf("type = %q", request.Type)
	}
	if request.EntityID != "comment-1" {
		t.Errorf("entity = %q", request.EntityID)
	}
	if request.ActionURL != "socialflow://posts/post-1#comment-comment-1" {
		t.Errorf("action URL = %q", request.ActionURL)
	}
}

func TestHandleMentionedWithoutCommentUsesPost(t *testing.T) {
	t.Parallel()

	service := &stubService{}
	handler := NewEventHandler(service, nil)

	event := newEvent(t, models.EventUserMentioned, models.UserMentionedEvent{
		MentionedUserID: uuid.NewString(),
		ActorID:         uuid.NewString(),
		PostID:          "post-1",
	}, "")

	if err := handler.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	request := service.processed[0]
	if request.EntityType != "post" || request.EntityID != "post-1" {
		t.Errorf("entity = %q/%q", request.EntityID, request.EntityType)
	}
}

func TestHandleSecurityAlertBypassesPolicy(t *testing.T) {
	t.Parallel()

	service := &stubService{}
	handler := NewEventHandler(service, nil)

	recipeint := uuid.New()
	event := newEvent(t, models.EventSecurityAlert, models.SecurityAlertEvent{
		UserID:  recipeint.String(),
		Title:   "New login",
		Message: "New device detected",
	}, "")

	if err := handler.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	request := service.processed[0]
	if request.Type != models.TypeSystemAlert {
		t.Errorf("type = %q", request.Type)
	}
	if !request.BypassPolicy {
		t.Error("security alerts must bypass preferences")
	}
	if request.TitleOverride != "New login" || request.BodyOverride != "New device detected" {
		t.Errorf("overrides = %q / %q", request.TitleOverride, request.BodyOverride)
	}
}

func TestHandleUnsupportedEventIsPermanent(t *testing.T) {
	t.Parallel()

	handler := NewEventHandler(&stubService{}, nil)
	event := newEvent(t, models.EventType("post.archived"), map[string]string{"post_id": "p1"}, "")

	err := handler.HandleEvent(context.Background(), event)
	if !errors.Is(err, models.ErrPermanentEvent) {
		t.Fatalf("expected ErrPermanentEvent, got %v", err)
	}
}

func TestHandleInvalidPayloadIsPermanent(t *testing.T) {
	t.Parallel()

	handler := NewEventHandler(&stubService{}, nil)
	event := models.Event{Type: models.EventPostLiked, Payload: json.RawMessage(`{"post_id":`)}
	if err := handler.HandleEvent(context.Background(), event); !errors.Is(err, models.ErrPermanentEvent) {
		t.Fatalf("expected ErrPermanentEvent, got %v", err)
	}
}

func TestHandleMissingIDsIsPermanent(t *testing.T) {
	t.Parallel()

	handler := NewEventHandler(&stubService{}, nil)
	event := newEvent(t, models.EventUserFollowed, models.UserFollowedEvent{}, "")

	if err := handler.HandleEvent(context.Background(), event); !errors.Is(err, models.ErrPermanentEvent) {
		t.Fatalf("expected ErrPermanentEvent, got %v", err)
	}
}

func TestHandleUnfollowedIsNoop(t *testing.T) {
	t.Parallel()

	service := &stubService{}
	handler := NewEventHandler(service, nil)
	event := newEvent(t, models.EventUserUnfollowed, models.UserUnfollowedEvent{
		FollowerID:  uuid.NewString(),
		FollowingID: uuid.NewString(),
	}, "")

	if err := handler.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if len(service.processed) != 0 || len(service.revoked) != 0 {
		t.Errorf("unfollow should be a no-op, processed=%d revoked=%d", len(service.processed), len(service.revoked))
	}
}
