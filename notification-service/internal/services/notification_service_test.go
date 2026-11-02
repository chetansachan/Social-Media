package services

import (
	"context"
	"errors"
	"log"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/chetansachan/socialflow/notification-service/internal/repository"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// -----------------------------------------------------------------------------
// Fakes
// -----------------------------------------------------------------------------

type fakeNotificationStore struct {
	mu    sync.Mutex
	items map[uuid.UUID]*models.Notification
}

func newFakeNotificationStore() *fakeNotificationStore {
	return &fakeNotificationStore{items: make(map[uuid.UUID]*models.Notification)}
}

func (s *fakeNotificationStore) Create(ctx context.Context, input *models.CreateNotificationInput) (*models.Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	notification := &models.Notification{
		ID:               uuid.New(),
		RecipientID:      input.RecipientID,
		Type:             input.Type,
		EntityID:         input.EntityID,
		EntityType:       input.EntityType,
		ActorIDs:         append([]string{}, input.ActorIDs...),
		TotalActorsCount: input.TotalActorsCount,
		Title:            input.Title,
		ContentPreview:   input.ContentPreview,
		ActionURL:        input.ActionURL,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	s.items[notification.ID] = notification

	copied := *notification
	return &copied, nil
}

func (s *fakeNotificationStore) GetByID(ctx context.Context, id uuid.UUID) (*models.Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	notification, ok := s.items[id]
	if !ok || notification.DeletedAt != nil {
		return nil, models.ErrNotificationNotFound
	}
	copied := *notification
	return &copied, nil
}

func (s *fakeNotificationStore) List(ctx context.Context, recipientID uuid.UUID, filter models.NotificationType, cursor models.Cursor, limit int) ([]models.Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]models.Notification, 0, len(s.items))
	for _, notification := range s.items {
		if notification.RecipientID != recipientID || notification.DeletedAt != nil {
			continue
		}
		if filter != "" && notification.Type != filter {
			continue
		}
		if !cursor.IsZero() {
			if notification.CreatedAt.After(cursor.CreatedAt) {
				continue
			}
			if notification.CreatedAt.Equal(cursor.CreatedAt) && notification.ID.String() >= cursor.ID.String() {
				continue
			}
		}
		result = append(result, *notification)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID.String() > result[j].ID.String()
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *fakeNotificationStore) MarkRead(ctx context.Context, recipientID uuid.UUID, ids []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	changed := 0
	for _, raw := range ids {
		id, err := uuid.Parse(raw)
		if err != nil {
			continue
		}
		notification, ok := s.items[id]
		if !ok || notification.RecipientID != recipientID || notification.IsRead || notification.DeletedAt != nil {
			continue
		}
		notification.IsRead = true
		now := time.Now().UTC()
		notification.ReadAt = &now
		changed++
	}
	return changed, nil
}

func (s *fakeNotificationStore) MarkAllRead(ctx context.Context, recipientID uuid.UUID) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	changed := 0
	for _, notification := range s.items {
		if notification.RecipientID != recipientID || notification.IsRead || notification.DeletedAt != nil {
			continue
		}
		notification.IsRead = true
		now := time.Now().UTC()
		notification.ReadAt = &now
		changed++
	}
	return changed, nil
}

func (s *fakeNotificationStore) Delete(ctx context.Context, recipientID uuid.UUID, id uuid.UUID) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	notification, ok := s.items[id]
	if !ok || notification.RecipientID != recipientID || notification.DeletedAt != nil {
		return false, models.ErrNotificationNotFound
	}
	now := time.Now().UTC()
	notification.DeletedAt = &now
	return !notification.IsRead, nil
}

func (s *fakeNotificationStore) CountUnread(ctx context.Context, recipientID uuid.UUID) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, notification := range s.items {
		if notification.RecipientID == recipientID && !notification.IsRead && notification.DeletedAt == nil {
			count++
		}
	}
	return count, nil
}

func (s *fakeNotificationStore) UpdateAggregate(ctx context.Context, id uuid.UUID, actorIDs []string, total int, title, preview string, reopen bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	notification, ok := s.items[id]
	if !ok || notification.DeletedAt != nil {
		return false, models.ErrNotificationNotFound
	}

	wasRead := notification.IsRead
	notification.ActorIDs = append([]string{}, actorIDs...)
	notification.TotalActorsCount = total
	notification.Title = title
	notification.ContentPreview = preview
	notification.UpdatedAt = time.Now().UTC()
	if reopen {
		notification.IsRead = false
		notification.ReadAt = nil
	}
	return wasRead, nil
}

func (s *fakeNotificationStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

func (s *fakeNotificationStore) first() *models.Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, notification := range s.items {
		copied := *notification
		return &copied
	}
	return nil
}

type fakeSettingsStore struct {
	mu       sync.Mutex
	settings map[uuid.UUID]*models.NotificationSettings
}

func newFakeSettingsStore() *fakeSettingsStore {
	return &fakeSettingsStore{settings: make(map[uuid.UUID]*models.NotificationSettings)}
}

func (s *fakeSettingsStore) Get(ctx context.Context, userID uuid.UUID) (*models.NotificationSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if settings, ok := s.settings[userID]; ok {
		copied := *settings
		return &copied, nil
	}
	return models.DefaultSettings(userID), nil
}

func (s *fakeSettingsStore) Save(ctx context.Context, settings *models.NotificationSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	copied := *settings
	s.settings[settings.UserID] = &copied
	return nil
}

type fakeActors struct{}

func (fakeActors) GetActors(ctx context.Context, ids []string) map[string]models.Actor {
	result := make(map[string]models.Actor, len(ids))
	for _, id := range ids {
		result[id] = models.Actor{
			ID:          id,
			Username:    "user-" + id[:8],
			DisplayName: "Alice",
			Email:       "alice@example.com",
		}
	}
	return result
}

type fakeRealtime struct {
	mu       sync.Mutex
	payloads []models.RealtimePayload
}

func (r *fakeRealtime) Publish(ctx context.Context, userID string, payload models.RealtimePayload) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.payloads = append(r.payloads, payload)
	return nil
}

func (r *fakeRealtime) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.payloads)
}

type fakePushQueue struct {
	mu       sync.Mutex
	messages []models.PushMessage
}

func (q *fakePushQueue) Enqueue(ctx context.Context, message models.PushMessage) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.messages = append(q.messages, message)
	return nil
}

func (q *fakePushQueue) count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.messages)
}

type fakeEmailQueue struct {
	mu       sync.Mutex
	messages []models.EmailMessage
}

func (q *fakeEmailQueue) Enqueue(ctx context.Context, message models.EmailMessage) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.messages = append(q.messages, message)
	return nil
}

func (q *fakeEmailQueue) count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.messages)
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

type testHarness struct {
	service  NotificationService
	store    *fakeNotificationStore
	settings *fakeSettingsStore
	realtime *fakeRealtime
	push     *fakePushQueue
	email    *fakeEmailQueue
	cache    repository.CacheRepository
	redis    *miniredis.Miniredis
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	cache := repository.NewCacheRepository(client, time.Hour)
	store := newFakeNotificationStore()
	settings := newFakeSettingsStore()
	realtime := &fakeRealtime{}
	push := &fakePushQueue{}
	email := &fakeEmailQueue{}

	service := NewNotificationService(Dependencies{
		Notifications: store,
		Settings:      settings,
		Cache:         cache,
		Actors:        fakeActors{},
		Realtime:      realtime,
		Push:          push,
		Email:         email,
		Options:       DefaultOptions(),
		Logger:        log.New(discardWriter{}, "", 0),
	})

	return &testHarness{
		service:  service,
		store:    store,
		settings: settings,
		realtime: realtime,
		push:     push,
		email:    email,
		cache:    cache,
		redis:    server,
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func likeRequest(recipient, actor uuid.UUID) *models.NotificationRequest {
	return &models.NotificationRequest{
		RecipientID:      recipient,
		ActorID:          &actor,
		ActorDisplayName: "Alice",
		Type:             models.TypePostLike,
		EntityID:         "post-1",
		EntityType:       "post",
	}
}

// -----------------------------------------------------------------------------
// Tests
// -----------------------------------------------------------------------------

func TestProcessEventCreatesNotification(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	recipient := uuid.New()
	actor := uuid.New()

	request := likeRequest(recipient, actor)
	request.ActorDisplayName = ""
	if err := h.service.ProcessEvent(context.Background(), request); err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}

	if h.store.count() != 1 {
		t.Fatalf("notification count = %d, want 1", h.store.count())
	}
	notification := h.store.first()
	if notification.Type != models.TypePostLike {
		t.Errorf("type = %q", notification.Type)
	}
	if notification.Title != "Alice liked your post" {
		t.Errorf("title = %q", notification.Title)
	}
	if notification.TotalActorsCount != 1 {
		t.Errorf("actor count = %d", notification.TotalActorsCount)
	}

	// push_likes defaults to false, so only the realtime frame is emitted.
	if h.realtime.count() != 1 {
		t.Errorf("realtime frames = %d, want 1", h.realtime.count())
	}
	if h.push.count() != 0 {
		t.Errorf("push messages = %d, want 0", h.push.count())
	}

	unread, err := h.service.GetUnreadCount(context.Background(), recipient.String())
	if err != nil || unread != 1 {
		t.Fatalf("unread = %d, %v", unread, err)
	}
}

func TestUnreadCounterRebuildsAfterCacheLoss(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	recipient := uuid.New()

	if err := h.service.ProcessEvent(context.Background(), likeRequest(recipient, uuid.New())); err != nil {
		t.Fatalf("first event: %v", err)
	}

	// Simulate a Redis restart/eviction. The next event must seed the badge from
	// PostgreSQL instead of incrementing from zero and losing the first unread.
	h.redis.FlushAll()

	second := &models.NotificationRequest{
		RecipientID: recipient,
		ActorID:     uuidPointer(uuid.New()),
		Type:        models.TypeFollow,
		EntityID:    "user-2",
		EntityType:  "user",
	}
	if err := h.service.ProcessEvent(context.Background(), second); err != nil {
		t.Fatalf("second event: %v", err)
	}

	unread, err := h.service.GetUnreadCount(context.Background(), recipient.String())
	if err != nil {
		t.Fatalf("GetUnreadCount: %v", err)
	}
	if unread != 2 {
		t.Fatalf("unread after cache loss = %d, want 2", unread)
	}
}

func TestProcessEventDropsSelfAction(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	userID := uuid.New()

	request := likeRequest(userID, userID)
	if err := h.service.ProcessEvent(context.Background(), request); err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}
	if h.store.count() != 0 {
		t.Fatalf("self action created %d notifications", h.store.count())
	}
}

func TestProcessEventDeduplicates(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	recipient := uuid.New()
	actor := uuid.New()
	request := likeRequest(recipient, actor)

	for i := 0; i < 3; i++ {
		if err := h.service.ProcessEvent(context.Background(), request); err != nil {
			t.Fatalf("ProcessEvent %d: %v", i, err)
		}
	}

	if h.store.count() != 1 {
		t.Fatalf("notification count = %d, want 1", h.store.count())
	}
}

func TestProcessEventAggregatesLikes(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	recipient := uuid.New()
	actorOne := uuid.New()
	actorTwo := uuid.New()

	if err := h.service.ProcessEvent(context.Background(), likeRequest(recipient, actorOne)); err != nil {
		t.Fatalf("first like: %v", err)
	}
	if err := h.service.ProcessEvent(context.Background(), likeRequest(recipient, actorTwo)); err != nil {
		t.Fatalf("second like: %v", err)
	}

	if h.store.count() != 1 {
		t.Fatalf("aggregation created %d rows, want 1", h.store.count())
	}
	notification := h.store.first()
	if notification.TotalActorsCount != 2 {
		t.Errorf("total actors = %d, want 2", notification.TotalActorsCount)
	}
	if len(notification.ActorIDs) != 2 {
		t.Errorf("actor ids = %v", notification.ActorIDs)
	}

	unread, err := h.service.GetUnreadCount(context.Background(), recipient.String())
	if err != nil || unread != 1 {
		t.Fatalf("unread = %d, %v", unread, err)
	}
}

func TestProcessEventDoesNotDoubleCountSameActor(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	recipient := uuid.New()
	actor := uuid.New()
	request := likeRequest(recipient, actor)

	if err := h.service.ProcessEvent(context.Background(), request); err != nil {
		t.Fatalf("first like: %v", err)
	}

	// Expire the short dedup lock but keep the aggregation window, simulating a
	// duplicate delivery after DEDUP_TTL.
	if err := h.cache.ReleaseDedup(context.Background(), request.DedupKey()); err != nil {
		t.Fatalf("release dedup: %v", err)
	}
	if err := h.service.ProcessEvent(context.Background(), request); err != nil {
		t.Fatalf("duplicate like: %v", err)
	}

	notification := h.store.first()
	if notification.TotalActorsCount != 1 {
		t.Errorf("total actors = %d, want 1", notification.TotalActorsCount)
	}
}

func TestRevokeAggregatedActor(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	recipient := uuid.New()
	actorOne := uuid.New()
	actorTwo := uuid.New()

	if err := h.service.ProcessEvent(context.Background(), likeRequest(recipient, actorOne)); err != nil {
		t.Fatalf("first like: %v", err)
	}
	if err := h.service.ProcessEvent(context.Background(), likeRequest(recipient, actorTwo)); err != nil {
		t.Fatalf("second like: %v", err)
	}

	revoke := likeRequest(recipient, actorTwo)
	if err := h.service.RevokeAggregatedActor(context.Background(), revoke); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	notification := h.store.first()
	if notification.TotalActorsCount != 1 {
		t.Errorf("total actors after revoke = %d, want 1", notification.TotalActorsCount)
	}
	if notification.DeletedAt != nil {
		t.Error("notification should still exist with one actor")
	}

	revoke = likeRequest(recipient, actorOne)
	if err := h.service.RevokeAggregatedActor(context.Background(), revoke); err != nil {
		t.Fatalf("second revoke: %v", err)
	}

	notification = h.store.first()
	if notification.DeletedAt == nil {
		t.Error("notification should be deleted when the last actor is revoked")
	}

	unread, err := h.service.GetUnreadCount(context.Background(), recipient.String())
	if err != nil || unread != 0 {
		t.Fatalf("unread after revoke = %d, %v", unread, err)
	}
}

func TestMarkAsReadUpdatesBadge(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	recipient := uuid.New()

	first := likeRequest(recipient, uuid.New())
	first.Type = models.TypeFollow
	first.EntityID = "user-1"
	second := likeRequest(recipient, uuid.New())
	second.Type = models.TypeComment
	second.EntityID = "comment-1"

	if err := h.service.ProcessEvent(context.Background(), first); err != nil {
		t.Fatalf("first event: %v", err)
	}
	if err := h.service.ProcessEvent(context.Background(), second); err != nil {
		t.Fatalf("second event: %v", err)
	}

	items, err := h.store.List(context.Background(), recipient, "", models.Cursor{}, 10)
	if err != nil || len(items) != 2 {
		t.Fatalf("list = %d items, %v", len(items), err)
	}

	remaining, err := h.service.MarkAsRead(context.Background(), recipient.String(), []string{items[0].ID.String()})
	if err != nil {
		t.Fatalf("MarkAsRead: %v", err)
	}
	if remaining != 1 {
		t.Errorf("remaining after mark one = %d, want 1", remaining)
	}

	marked, err := h.service.MarkAllAsRead(context.Background(), recipient.String())
	if err != nil {
		t.Fatalf("MarkAllAsRead: %v", err)
	}
	if marked != 1 {
		t.Errorf("marked count = %d, want 1", marked)
	}

	unread, err := h.service.GetUnreadCount(context.Background(), recipient.String())
	if err != nil || unread != 0 {
		t.Fatalf("unread = %d, %v", unread, err)
	}
}

func TestDeleteNotificationDecrementsBadge(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	recipient := uuid.New()
	actor := uuid.New()

	request := likeRequest(recipient, actor)
	if err := h.service.ProcessEvent(context.Background(), request); err != nil {
		t.Fatalf("process: %v", err)
	}

	notification := h.store.first()
	if err := h.service.DeleteNotification(context.Background(), recipient.String(), notification.ID.String()); err != nil {
		t.Fatalf("delete: %v", err)
	}

	unread, err := h.service.GetUnreadCount(context.Background(), recipient.String())
	if err != nil || unread != 0 {
		t.Fatalf("unread = %d, %v", unread, err)
	}
}

func TestQuietHoursSuppressPush(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	recipient := uuid.New()
	actor := uuid.New()

	settings := models.DefaultSettings(recipient)
	settings.PushFollows = true
	settings.QuietHoursEnabled = true
	settings.QuietHoursStart = "00:00"
	settings.QuietHoursEnd = "00:00" // all day
	_ = h.settings.Save(context.Background(), settings)

	request := &models.NotificationRequest{
		RecipientID: recipient,
		ActorID:     &actor,
		Type:        models.TypeFollow,
		EntityID:    "actor-1",
		EntityType:  "user",
	}
	if err := h.service.ProcessEvent(context.Background(), request); err != nil {
		t.Fatalf("process: %v", err)
	}

	if h.realtime.count() != 1 {
		t.Errorf("realtime frames = %d, want 1", h.realtime.count())
	}
	if h.push.count() != 0 {
		t.Errorf("push messages = %d, want 0 during quiet hours", h.push.count())
	}
}

func TestSecurityAlertBypassesPolicyAndEmails(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	recipient := uuid.New()

	settings := models.DefaultSettings(recipient)
	settings.PushEnabled = false
	settings.EmailSecurityAlerts = true
	settings.QuietHoursEnabled = true
	settings.QuietHoursStart = "00:00"
	settings.QuietHoursEnd = "00:00"
	_ = h.settings.Save(context.Background(), settings)

	request := &models.NotificationRequest{
		RecipientID:   recipient,
		Type:          models.TypeSystemAlert,
		EntityID:      "system",
		EntityType:    "system",
		TitleOverride: "New login",
		BodyOverride:  "A new device signed in.",
		ActionURL:     "socialflow://settings/security",
		BypassPolicy:  true,
	}
	if err := h.service.ProcessEvent(context.Background(), request); err != nil {
		t.Fatalf("process: %v", err)
	}

	if h.store.count() != 1 {
		t.Fatalf("system alert count = %d", h.store.count())
	}
	if h.push.count() != 1 {
		t.Errorf("security alerts must bypass push policy, got %d messages", h.push.count())
	}
	if h.email.count() != 1 {
		t.Errorf("security alert email count = %d, want 1", h.email.count())
	}
}

func TestGetNotificationsPagination(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	recipient := uuid.New()

	for i := 0; i < 3; i++ {
		request := &models.NotificationRequest{
			RecipientID: recipient,
			ActorID:     uuidPointer(uuid.New()),
			Type:        models.TypeFollow,
			EntityID:    uuid.NewString(),
			EntityType:  "user",
		}
		if err := h.service.ProcessEvent(context.Background(), request); err != nil {
			t.Fatalf("process %d: %v", i, err)
		}
	}

	page, err := h.service.GetNotifications(context.Background(), recipient.String(), 2, "", "")
	if err != nil {
		t.Fatalf("GetNotifications: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("page size = %d, want 2", len(page.Items))
	}
	if !page.HasMore {
		t.Error("expected has_more on first page")
	}
	if page.NextCursor == "" {
		t.Error("expected a next cursor")
	}

	second, err := h.service.GetNotifications(context.Background(), recipient.String(), 2, page.NextCursor, "")
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Items) != 1 {
		t.Fatalf("second page size = %d, want 1", len(second.Items))
	}
	if second.HasMore {
		t.Error("second page should not have more")
	}
}

func TestGetNotificationsFilter(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	recipient := uuid.New()
	actor := uuid.New()

	follow := &models.NotificationRequest{RecipientID: recipient, ActorID: &actor, Type: models.TypeFollow, EntityID: "user-1"}
	comment := &models.NotificationRequest{RecipientID: recipient, ActorID: &actor, Type: models.TypeComment, EntityID: "comment-1"}
	for _, request := range []*models.NotificationRequest{follow, comment} {
		if err := h.service.ProcessEvent(context.Background(), request); err != nil {
			t.Fatalf("process: %v", err)
		}
	}

	page, err := h.service.GetNotifications(context.Background(), recipient.String(), 10, "", "post_like")
	if err != nil {
		t.Fatalf("GetNotifications: %v", err)
	}
	if len(page.Items) != 0 {
		t.Errorf("filtered items = %d, want 0", len(page.Items))
	}
}

func TestUpdateSettingsRejectsForeignUserID(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	pathUser := uuid.New()
	bodyUser := uuid.New()

	settings := models.DefaultSettings(bodyUser)
	updated, err := h.service.UpdateSettings(context.Background(), pathUser.String(), settings)
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if updated.UserID != pathUser {
		t.Errorf("user id = %s, want path user %s", updated.UserID, pathUser)
	}
}

func TestDeviceTokenValidation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	userID := uuid.New()

	if err := h.service.RegisterDeviceToken(context.Background(), userID.String(), "", "ios", "", ""); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("empty token error = %v", err)
	}
	if err := h.service.RegisterDeviceToken(context.Background(), userID.String(), "token", "windows", "", ""); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("bad platform error = %v", err)
	}
}

func uuidPointer(id uuid.UUID) *uuid.UUID {
	return &id
}
