package events

import (
	"context"
	"errors"
	"testing"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/google/uuid"
)

func TestHandleRepliedNotifiesParentAuthor(t *testing.T) {
	t.Parallel()

	service := &stubService{}
	handler := NewEventHandler(service, nil)

	parentAuthor := uuid.New()
	actor := uuid.New()
	event := newEvent(t, models.EventCommentReplied, models.CommentRepliedEvent{
		PostID:         "post-1",
		CommentID:      "comment-2",
		ParentAuthorID: parentAuthor.String(),
		ActorID:        actor.String(),
		ContentPreview: "same here",
	}, "evt-reply")

	if err := handler.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if len(service.processed) != 1 {
		t.Fatalf("processed = %d, want 1", len(service.processed))
	}

	request := service.processed[0]
	// The recipient is the author of the comment being replied to, not the
	// author of the post the thread hangs off.
	if request.Type != models.TypeCommentReply {
		t.Errorf("type = %q", request.Type)
	}
	if request.RecipientID != parentAuthor || request.ActorID == nil || *request.ActorID != actor {
		t.Errorf("recipient/actor mismatch: %+v", request)
	}
	if request.EntityID != "comment-2" || request.EntityType != "comment" {
		t.Errorf("entity = %q/%q", request.EntityID, request.EntityType)
	}
	// A reply deep-links into the thread, so both the post and the comment id
	// are needed to build the URL.
	if request.ActionURL != "socialflow://posts/post-1#comment-comment-2" {
		t.Errorf("action URL = %q", request.ActionURL)
	}
	if request.ContentPreview != "same here" {
		t.Errorf("preview = %q", request.ContentPreview)
	}
	if request.EventID != "evt-reply" {
		t.Errorf("event_id = %q", request.EventID)
	}
}

func TestHandleRepliedWithoutParentAuthorIsPermanent(t *testing.T) {
	t.Parallel()

	service := &stubService{}
	handler := NewEventHandler(service, nil)
	event := newEvent(t, models.EventCommentReplied, models.CommentRepliedEvent{
		PostID:    "post-1",
		CommentID: "comment-2",
		ActorID:   uuid.NewString(),
	}, "")

	// Without the parent author there is nobody to notify, and retrying the
	// producer's event will never add the missing field.
	if err := handler.HandleEvent(context.Background(), event); !errors.Is(err, models.ErrPermanentEvent) {
		t.Fatalf("expected ErrPermanentEvent, got %v", err)
	}
	if len(service.processed) != 0 {
		t.Errorf("no notification should be created, got %d", len(service.processed))
	}
}

func TestHandleRepliedWithoutCommentIsPermanent(t *testing.T) {
	t.Parallel()

	service := &stubService{}
	handler := NewEventHandler(service, nil)
	event := newEvent(t, models.EventCommentReplied, models.CommentRepliedEvent{
		PostID:         "post-1",
		ParentAuthorID: uuid.NewString(),
		ActorID:        uuid.NewString(),
	}, "")

	// The reply is the entity being notified about, so a blank comment id
	// cannot be deep-linked and must not be retried forever.
	if err := handler.HandleEvent(context.Background(), event); !errors.Is(err, models.ErrPermanentEvent) {
		t.Fatalf("expected ErrPermanentEvent, got %v", err)
	}
	if len(service.processed) != 0 {
		t.Errorf("no notification should be created, got %d", len(service.processed))
	}
}

func TestHandleCreatorPostNotifiesSubscriber(t *testing.T) {
	t.Parallel()

	service := &stubService{}
	handler := NewEventHandler(service, nil)

	author := uuid.New()
	subscriber := uuid.New()
	event := newEvent(t, models.EventCreatorPost, models.CreatorPostEvent{
		PostID:           "post-1",
		AuthorID:         author.String(),
		RecipientID:      subscriber.String(),
		ActorUsername:    "creator",
		ActorDisplayName: "The Creator",
		ActorAvatarURL:   "https://cdn/avatar.png",
		PostPreview:      "a new post",
	}, "evt-post")

	if err := handler.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	request := service.processed[0]
	// Fan-out is the producer's job: one event reaches exactly one follower,
	// which is why a single recipient id travels with the payload.
	if request.Type != models.TypeCreatorPost {
		t.Errorf("type = %q", request.Type)
	}
	if request.RecipientID != subscriber || request.ActorID == nil || *request.ActorID != author {
		t.Errorf("recipient/actor mismatch: %+v", request)
	}
	if request.ActorUsername != "creator" || request.ActorDisplayName != "The Creator" {
		t.Errorf("actor profile = %q/%q", request.ActorUsername, request.ActorDisplayName)
	}
	if request.EntityID != "post-1" || request.EntityType != "post" {
		t.Errorf("entity = %q/%q", request.EntityID, request.EntityType)
	}
	if request.ActionURL != "socialflow://posts/post-1" {
		t.Errorf("action URL = %q", request.ActionURL)
	}
	if request.ContentPreview != "a new post" {
		t.Errorf("preview = %q", request.ContentPreview)
	}
	if request.EventID != "evt-post" {
		t.Errorf("event_id = %q", request.EventID)
	}
}

func TestHandleCreatorPostWithoutRecipientIsPermanent(t *testing.T) {
	t.Parallel()

	handler := NewEventHandler(&stubService{}, nil)
	event := newEvent(t, models.EventCreatorPost, models.CreatorPostEvent{
		PostID:   "post-1",
		AuthorID: uuid.NewString(),
	}, "")

	// The producer forgot the follower this fan-out batch was addressed to.
	if err := handler.HandleEvent(context.Background(), event); !errors.Is(err, models.ErrPermanentEvent) {
		t.Fatalf("expected ErrPermanentEvent, got %v", err)
	}
}

func TestHandleCreatorPostWithoutAuthorIsPermanent(t *testing.T) {
	t.Parallel()

	handler := NewEventHandler(&stubService{}, nil)
	event := newEvent(t, models.EventCreatorPost, models.CreatorPostEvent{
		PostID:      "post-1",
		RecipientID: uuid.NewString(),
	}, "")

	// The author is the actor of the notification; a blank or unparsable id
	// cannot be stored, so the event is dropped instead of retried.
	if err := handler.HandleEvent(context.Background(), event); !errors.Is(err, models.ErrPermanentEvent) {
		t.Fatalf("expected ErrPermanentEvent, got %v", err)
	}
}

func TestHandleCreatorPostWithoutPostIsPermanent(t *testing.T) {
	t.Parallel()

	handler := NewEventHandler(&stubService{}, nil)
	event := newEvent(t, models.EventCreatorPost, models.CreatorPostEvent{
		RecipientID: uuid.NewString(),
		AuthorID:    uuid.NewString(),
	}, "")

	// A post id that is missing or only whitespace has no action URL to
	// deep-link to.
	if err := handler.HandleEvent(context.Background(), event); !errors.Is(err, models.ErrPermanentEvent) {
		t.Fatalf("expected ErrPermanentEvent, got %v", err)
	}
}

func TestHandleWelcomeUsesDefaultsForMissingCopy(t *testing.T) {
	t.Parallel()

	service := &stubService{}
	handler := NewEventHandler(service, nil)

	user := uuid.New()
	event := newEvent(t, models.EventWelcome, models.WelcomeMessageEvent{UserID: user.String()}, "evt-welcome")

	if err := handler.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	request := service.processed[0]
	if request.RecipientID != user {
		t.Errorf("recipient = %s, want %s", request.RecipientID, user)
	}
	// The welcome message is mandatory, so blank copy falls back to server
	// defaults rather than rendering an empty notification.
	if request.Type != models.TypeSystemAlert {
		t.Errorf("type = %q", request.Type)
	}
	if request.TitleOverride != "Welcome to SocialFlow" {
		t.Errorf("title = %q", request.TitleOverride)
	}
	if request.BodyOverride != "Your account is ready. Follow a few people to start building your feed." {
		t.Errorf("body = %q", request.BodyOverride)
	}
	if request.ActionURL != "socialflow://explore" {
		t.Errorf("action URL = %q", request.ActionURL)
	}
	// Onboarding copy is never suppressed by user preferences.
	if !request.BypassPolicy {
		t.Error("the welcome message must bypass preferences")
	}
	if request.EntityID != "system" || request.EntityType != "system" {
		t.Errorf("entity = %q/%q", request.EntityID, request.EntityType)
	}
	if request.EventID != "evt-welcome" {
		t.Errorf("event_id = %q", request.EventID)
	}
}

func TestHandleWelcomeAcceptsRecipientSpellings(t *testing.T) {
	t.Parallel()

	user := uuid.New()
	service := &stubService{}
	handler := NewEventHandler(service, nil)

	// Producers spell the audience as user_id or recipient_id; both resolve.
	event := newEvent(t, models.EventWelcome, models.WelcomeMessageEvent{
		RecipientID: user.String(),
		Title:       "Welcome aboard",
		Message:     "Your account is ready.",
		ActionURL:   "socialflow://explore",
	}, "")

	if err := handler.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	request := service.processed[0]
	if request.TitleOverride != "Welcome aboard" || request.BodyOverride != "Your account is ready." {
		t.Errorf("copy = %q/%q", request.TitleOverride, request.BodyOverride)
	}
	if request.ActionURL != "socialflow://explore" {
		t.Errorf("action URL = %q", request.ActionURL)
	}
}

func TestHandleWelcomeWithoutRecipientIsPermanent(t *testing.T) {
	t.Parallel()

	handler := NewEventHandler(&stubService{}, nil)
	event := newEvent(t, models.EventWelcome, models.WelcomeMessageEvent{
		Title:   "Welcome to SocialFlow",
		Message: "Your account is ready.",
	}, "")

	if err := handler.HandleEvent(context.Background(), event); !errors.Is(err, models.ErrPermanentEvent) {
		t.Fatalf("expected ErrPermanentEvent, got %v", err)
	}
}

func TestReactionIDsResolvesRecipientSpellings(t *testing.T) {
	t.Parallel()

	author := uuid.New()
	actor := uuid.New()

	tests := []struct {
		name          string
		payload       models.PostReactionEvent
		wantRecipient uuid.UUID
	}{
		{
			name:          "post author id",
			payload:       models.PostReactionEvent{PostID: "post-1", PostAuthorID: author.String(), ActorID: actor.String()},
			wantRecipient: author,
		},
		{
			name:          "recipient id fallback",
			payload:       models.PostReactionEvent{PostID: "post-1", RecipientID: author.String(), ActorID: actor.String()},
			wantRecipient: author,
		},
		{
			name:          "user id fallback",
			payload:       models.PostReactionEvent{PostID: "post-1", UserID: author.String(), ActorID: actor.String()},
			wantRecipient: author,
		},
		{
			// The most specific spelling wins so a producer that sends both
			// does not accidentally notify the wrong user.
			name:          "author id wins over user id",
			payload:       models.PostReactionEvent{PostID: "post-1", PostAuthorID: author.String(), UserID: actor.String(), ActorID: actor.String()},
			wantRecipient: author,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recipient, gotActor, err := reactionIDs(test.payload)
			if err != nil {
				t.Fatalf("reactionIDs: %v", err)
			}
			if recipient != test.wantRecipient || gotActor != actor {
				t.Fatalf("recipient/actor = %s/%s, want %s/%s", recipient, gotActor, test.wantRecipient, actor)
			}
		})
	}
}

func TestReactionIDsRejectsIncompletePayloads(t *testing.T) {
	t.Parallel()

	author := uuid.New()
	actor := uuid.New()

	tests := []struct {
		name    string
		payload models.PostReactionEvent
	}{
		{
			// A like without a post has no entity to attach the notification to.
			name:    "missing post id",
			payload: models.PostReactionEvent{PostAuthorID: author.String(), ActorID: actor.String()},
		},
		{
			name:    "whitespace post id",
			payload: models.PostReactionEvent{PostID: "   ", PostAuthorID: author.String(), ActorID: actor.String()},
		},
		{
			// No post author means nobody to notify.
			name:    "missing recipient",
			payload: models.PostReactionEvent{PostID: "post-1", ActorID: actor.String()},
		},
		{
			name:    "invalid recipient",
			payload: models.PostReactionEvent{PostID: "post-1", PostAuthorID: "not-a-uuid", ActorID: actor.String()},
		},
		{
			// The actor is stored as a foreign key, so a bad id is dropped
			// rather than retried.
			name:    "missing actor",
			payload: models.PostReactionEvent{PostID: "post-1", PostAuthorID: author.String()},
		},
		{
			name:    "invalid actor",
			payload: models.PostReactionEvent{PostID: "post-1", PostAuthorID: author.String(), ActorID: "12"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := reactionIDs(test.payload)
			if !errors.Is(err, models.ErrPermanentEvent) {
				t.Fatalf("expected ErrPermanentEvent, got %v", err)
			}
		})
	}
}

func TestPostURLFallsBackToNotifications(t *testing.T) {
	t.Parallel()

	tests := []struct {
		postID string
		want   string
	}{
		{postID: "post-1", want: "socialflow://posts/post-1"},
		{postID: "", want: "socialflow://notifications"},
		{postID: "  ", want: "socialflow://notifications"},
	}

	for _, test := range tests {
		if got := postURL(test.postID); got != test.want {
			t.Errorf("postURL(%q) = %q, want %q", test.postID, got, test.want)
		}
	}
}

func TestCommentURLFallsBackToPostOrNotifications(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		postID    string
		commentID string
		want      string
	}{
		{name: "post and comment", postID: "post-1", commentID: "comment-1", want: "socialflow://posts/post-1#comment-comment-1"},
		{name: "comment trimmed away", postID: "post-1", commentID: " ", want: "socialflow://posts/post-1"},
		{name: "post only", postID: "post-1", want: "socialflow://posts/post-1"},
		{name: "no post", commentID: "comment-1", want: "socialflow://notifications"},
		{name: "neither", want: "socialflow://notifications"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := commentURL(test.postID, test.commentID); got != test.want {
				t.Errorf("commentURL(%q, %q) = %q, want %q", test.postID, test.commentID, got, test.want)
			}
		})
	}
}

func TestHandleRepliedFallbackURLsWithoutPost(t *testing.T) {
	t.Parallel()

	service := &stubService{}
	handler := NewEventHandler(service, nil)

	// Some producers omit the post id; the deep link then degrades to the
	// notification list instead of a broken "socialflow://posts/#comment-2".
	event := newEvent(t, models.EventCommentReplied, models.CommentRepliedEvent{
		CommentID:      "comment-2",
		ParentAuthorID: uuid.NewString(),
		ActorID:        uuid.NewString(),
	}, "")

	if err := handler.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if got := service.processed[0].ActionURL; got != "socialflow://notifications" {
		t.Errorf("action URL = %q, want the notifications fallback", got)
	}
}
