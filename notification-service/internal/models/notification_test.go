package models

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestParseNotificationType(t *testing.T) {
	t.Parallel()

	cases := map[string]NotificationType{
		"post_like":                   TypePostLike,
		"POST_LIKE":                   TypePostLike,
		"NOTIFICATION_TYPE_POST_LIKE": TypePostLike,
		"like":                        TypePostLike,
		"comment-reply":               TypeCommentReply,
		"notification_type_follow":    TypeFollow,
		"security_alert":              TypeSystemAlert,
	}

	for input, want := range cases {
		got, err := ParseNotificationType(input)
		if err != nil {
			t.Fatalf("ParseNotificationType(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Errorf("ParseNotificationType(%q) = %q, want %q", input, got, want)
		}
	}

	if _, err := ParseNotificationType("not-a-type"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for unknown type, got %v", err)
	}
}

func TestNotificationTypeCategoryAndAggregation(t *testing.T) {
	t.Parallel()

	if TypePostLike.Category() != "likes" {
		t.Errorf("post_like category = %q, want likes", TypePostLike.Category())
	}
	if !TypePostLike.Aggregatable() {
		t.Error("post_like should be aggregatable")
	}
	if TypeComment.Aggregatable() {
		t.Error("comment should not be aggregatable")
	}
	if TypeFollow.EntityTypeFor() != "user" {
		t.Errorf("follow entity type = %q, want user", TypeFollow.EntityTypeFor())
	}
}

func TestRenderTitle(t *testing.T) {
	t.Parallel()

	alice := Actor{ID: uuid.NewString(), DisplayName: "Alice"}
	bob := Actor{ID: uuid.NewString(), Username: "bob"}
	carol := Actor{ID: uuid.NewString(), Username: "carol"}

	cases := []struct {
		name   string
		actors []Actor
		total  int
		want   string
	}{
		{"single", []Actor{alice}, 1, "Alice liked your post"},
		{"pair", []Actor{alice, bob}, 2, "Alice and bob liked your post"},
		{"three", []Actor{alice, bob, carol}, 3, "Alice, bob, and carol liked your post"},
		{"many", []Actor{alice, bob, carol}, 12, "Alice, bob, carol, and 9 others liked your post"},
		{"two plus one", []Actor{alice, bob, carol}, 4, "Alice, bob, carol, and 1 other liked your post"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := RenderTitle(TypePostLike, tc.actors, tc.total); got != tc.want {
				t.Errorf("RenderTitle() = %q, want %q", got, tc.want)
			}
		})
	}

	if got := RenderTitle(TypeFollow, []Actor{alice}, 1); got != "Alice started following you" {
		t.Errorf("follow title = %q", got)
	}
}

func TestRenderPreview(t *testing.T) {
	t.Parallel()

	if got := RenderPreview(TypeFollow, ""); got != "You have a new follower" {
		t.Errorf("follow fallback = %q", got)
	}

	long := make([]rune, 300)
	for i := range long {
		long[i] = 'a'
	}
	got := RenderPreview(TypeComment, string(long))
	if len([]rune(got)) != 255 {
		t.Errorf("preview length = %d, want 255", len([]rune(got)))
	}
}

func TestNotificationRequestDedupKey(t *testing.T) {
	t.Parallel()

	recipient := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	actor := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	request := NotificationRequest{
		RecipientID: recipient,
		ActorID:     &actor,
		Type:        TypePostLike,
		EntityID:    "post-1",
	}
	want := "dedup:" + recipient.String() + ":" + actor.String() + ":post_like:post-1"
	if got := request.DedupKey(); got != want {
		t.Errorf("DedupKey() = %q, want %q", got, want)
	}

	request.EventID = "evt-123"
	if got := request.DedupKey(); got != "dedup:event:evt-123" {
		t.Errorf("event dedup key = %q", got)
	}
}

func TestNotificationRequestValidate(t *testing.T) {
	t.Parallel()

	recipient := uuid.New()
	actor := uuid.New()

	valid := NotificationRequest{RecipientID: recipient, ActorID: &actor, Type: TypeFollow, EntityID: "entity"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid request failed: %v", err)
	}

	self := NotificationRequest{RecipientID: recipient, ActorID: &recipient, Type: TypeFollow, EntityID: "entity"}
	if err := self.Validate(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("self action should be invalid, got %v", err)
	}

	missingEntity := NotificationRequest{RecipientID: recipient, Type: TypeFollow}
	if err := missingEntity.Validate(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing entity should be invalid, got %v", err)
	}

	system := NotificationRequest{RecipientID: recipient, Type: TypeSystemAlert}
	if err := system.Validate(); err != nil {
		t.Fatalf("system alert should not require entity_id: %v", err)
	}
}

func TestDefaultActionURL(t *testing.T) {
	t.Parallel()

	if got := DefaultActionURL(TypeFollow, "", "actor-1"); got != "socialflow://users/actor-1" {
		t.Errorf("follow URL = %q", got)
	}
	if got := DefaultActionURL(TypeComment, "post-1", "actor-1"); got != "socialflow://posts/post-1" {
		t.Errorf("comment URL = %q", got)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	t.Parallel()

	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	id := uuid.New()

	encoded := EncodeCursor(createdAt, id)
	decoded, err := DecodeCursor(encoded)
	if err != nil {
		t.Fatalf("DecodeCursor() error: %v", err)
	}
	if !decoded.CreatedAt.Equal(createdAt) {
		t.Errorf("created_at = %s, want %s", decoded.CreatedAt, createdAt)
	}
	if decoded.ID != id {
		t.Errorf("id = %s, want %s", decoded.ID, id)
	}

	if empty, err := DecodeCursor(""); err != nil || !empty.IsZero() {
		t.Fatalf("empty cursor should decode to zero value, got %+v / %v", empty, err)
	}

	if _, err := DecodeCursor("not-base64!"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("expected ErrInvalidCursor, got %v", err)
	}
}

func TestSettingsQuietHours(t *testing.T) {
	t.Parallel()

	settings := DefaultSettings(uuid.New())
	settings.QuietHoursEnabled = true
	settings.QuietHoursStart = "22:00"
	settings.QuietHoursEnd = "07:00"
	settings.Timezone = "UTC"

	at := func(hour, minute int) time.Time {
		return time.Date(2026, 3, 1, hour, minute, 0, 0, time.UTC)
	}

	if !settings.IsQuietHours(at(23, 0)) {
		t.Error("23:00 should be quiet")
	}
	if !settings.IsQuietHours(at(3, 0)) {
		t.Error("03:00 should be quiet")
	}
	if settings.IsQuietHours(at(12, 0)) {
		t.Error("12:00 should not be quiet")
	}

	settings.QuietHoursStart = "09:00"
	settings.QuietHoursEnd = "09:00"
	if !settings.IsQuietHours(at(12, 0)) {
		t.Error("equal start/end should mean all day")
	}
}

func TestSettingsValidate(t *testing.T) {
	t.Parallel()

	settings := DefaultSettings(uuid.New())
	if err := settings.Validate(); err != nil {
		t.Fatalf("default settings should validate: %v", err)
	}

	settings.Timezone = "Mars/Olympus"
	if err := settings.Validate(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid timezone should fail, got %v", err)
	}

	settings = DefaultSettings(uuid.New())
	settings.QuietHoursEnabled = true
	if err := settings.Validate(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("quiet hours without times should fail, got %v", err)
	}
}

func TestSettingsChannelToggles(t *testing.T) {
	t.Parallel()

	settings := DefaultSettings(uuid.New())
	if !settings.InAppEnabled(TypePostLike) {
		t.Error("in-app likes should default on")
	}
	if settings.PushCategoryEnabled(TypePostLike) {
		t.Error("push likes should default off")
	}

	settings.PushLikes = true
	if !settings.PushCategoryEnabled(TypePostLike) {
		t.Error("push likes should be on after enabling")
	}
	settings.PushEnabled = false
	if settings.PushCategoryEnabled(TypePostLike) {
		t.Error("master push switch must win")
	}

	if !settings.EmailCategoryEnabled(TypeSystemAlert) {
		t.Error("security alert email should default on")
	}
}
