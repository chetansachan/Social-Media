package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/google/uuid"
)

// NotificationService is the public orchestration contract consumed by the gRPC
// handler and the event handler.
type NotificationService interface {
	GetNotifications(ctx context.Context, userID string, pageSize int32, cursor string, filter string) (*models.NotificationPage, error)
	GetUnreadCount(ctx context.Context, userID string) (int32, error)
	MarkAsRead(ctx context.Context, userID string, ids []string) (int32, error)
	MarkAllAsRead(ctx context.Context, userID string) (int32, error)
	DeleteNotification(ctx context.Context, userID, notificationID string) error

	GetSettings(ctx context.Context, userID string) (*models.NotificationSettings, error)
	UpdateSettings(ctx context.Context, userID string, settings *models.NotificationSettings) (*models.NotificationSettings, error)

	RegisterDeviceToken(ctx context.Context, userID, token, platform, deviceName, appVersion string) error
	RevokeDeviceToken(ctx context.Context, userID, token string) error

	// ProcessEvent is the write path used by the stream consumer.
	ProcessEvent(ctx context.Context, request *models.NotificationRequest) error

	// RevokeAggregatedActor applies an unlike/unfollow to an existing roll-up.
	RevokeAggregatedActor(ctx context.Context, request *models.NotificationRequest) error
}

// Dependencies are wired once in cmd/server. Optional collaborators are
// tolerated so the service can run read-only or without Redis.
type Dependencies struct {
	Notifications NotificationStore
	Settings      SettingsStore
	Cache         CacheStore
	Devices       DeviceStore
	Actors        ActorResolver
	Aggregator    *Aggregator
	Policy        *PolicyService
	Realtime      RealtimePublisher
	Push          PushQueue
	Email         EmailQueue
	Options       Options
	Logger        *log.Logger
}

type notificationService struct {
	notifications NotificationStore
	settings      SettingsStore
	cache         CacheStore
	devices       DeviceStore
	actors        ActorResolver
	aggregator    *Aggregator
	policy        *PolicyService
	realtime      RealtimePublisher
	push          PushQueue
	email         EmailQueue
	options       Options
	logger        *log.Logger
}

func NewNotificationService(deps Dependencies) NotificationService {
	options := deps.Options.withDefaults()

	logger := deps.Logger
	if logger == nil {
		logger = log.Default()
	}

	aggregator := deps.Aggregator
	if aggregator == nil && deps.Cache != nil {
		aggregator = NewAggregator(deps.Cache, options.AggregationWindow, options.RealtimeThrottle)
	}

	policy := deps.Policy
	if policy == nil {
		policy = NewPolicyService(deps.Settings, deps.Cache, options)
	}

	return &notificationService{
		notifications: deps.Notifications,
		settings:      deps.Settings,
		cache:         deps.Cache,
		devices:       deps.Devices,
		actors:        deps.Actors,
		aggregator:    aggregator,
		policy:        policy,
		realtime:      deps.Realtime,
		push:          deps.Push,
		email:         deps.Email,
		options:       options,
		logger:        logger,
	}
}

// -----------------------------------------------------------------------------
// Inbox reads
// -----------------------------------------------------------------------------

func (s *notificationService) GetNotifications(ctx context.Context, userID string, pageSize int32, cursor string, filter string) (*models.NotificationPage, error) {
	recipientID, err := parseUserID(userID)
	if err != nil {
		return nil, err
	}

	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var filterType models.NotificationType
	if strings.TrimSpace(filter) != "" {
		filterType, err = models.ParseNotificationType(filter)
		if err != nil {
			return nil, err
		}
	}

	position, err := models.DecodeCursor(cursor)
	if err != nil {
		return nil, err
	}

	// Fetch one extra row to determine has_more without a COUNT query.
	notifications, err := s.notifications.List(ctx, recipientID, filterType, position, int(pageSize)+1)
	if err != nil {
		return nil, err
	}

	hasMore := len(notifications) > int(pageSize)
	if hasMore {
		notifications = notifications[:pageSize]
	}

	nextCursor := ""
	if hasMore && len(notifications) > 0 {
		last := notifications[len(notifications)-1]
		nextCursor = models.EncodeCursor(last.CreatedAt, last.ID)
	}

	s.hydrateNotifications(ctx, notifications)

	return &models.NotificationPage{
		Items:       notifications,
		NextCursor:  nextCursor,
		HasMore:     hasMore,
		UnreadCount: s.unreadCount(ctx, recipientID),
	}, nil
}

func (s *notificationService) GetUnreadCount(ctx context.Context, userID string) (int32, error) {
	recipientID, err := parseUserID(userID)
	if err != nil {
		return 0, err
	}
	return s.unreadCount(ctx, recipientID), nil
}

func (s *notificationService) unreadCount(ctx context.Context, userID uuid.UUID) int32 {
	if s.cache != nil {
		value, found, err := s.cache.GetUnread(ctx, userID)
		if err == nil && found {
			return clampInt32(value)
		}
	}

	count, err := s.notifications.CountUnread(ctx, userID)
	if err != nil {
		s.logf("failed to count unread notifications for %s: %v", userID, err)
		return 0
	}

	if s.cache != nil {
		_ = s.cache.SetUnread(ctx, userID, int64(count))
	}
	return clampInt32(int64(count))
}

func (s *notificationService) unreadAfterCreate(ctx context.Context, userID uuid.UUID) int64 {
	return s.incrementUnread(ctx, userID)
}

func (s *notificationService) unreadAfterAggregate(ctx context.Context, userID uuid.UUID, wasRead bool) int64 {
	if wasRead {
		return s.incrementUnread(ctx, userID)
	}

	if s.cache != nil {
		value, found, err := s.cache.GetUnread(ctx, userID)
		if err == nil && found {
			return value
		}
	}
	return s.databaseUnread(ctx, userID)
}

// incrementUnread only INCRs a counter that already exists. When the key was
// evicted (or has never been seeded), CountUnread already includes the row that
// was just committed, so seeding from the database is authoritative. Blindly
// incrementing from zero would silently reset a user's badge after a Redis
// restart.
func (s *notificationService) incrementUnread(ctx context.Context, userID uuid.UUID) int64 {
	if s.cache == nil {
		return s.databaseUnread(ctx, userID)
	}

	_, found, err := s.cache.GetUnread(ctx, userID)
	if err != nil {
		s.logf("failed to read unread counter for %s: %v", userID, err)
		return s.databaseUnread(ctx, userID)
	}
	if !found {
		return s.databaseUnread(ctx, userID)
	}

	value, err := s.cache.IncrUnread(ctx, userID)
	if err != nil {
		s.logf("failed to increment unread counter for %s: %v", userID, err)
		return s.databaseUnread(ctx, userID)
	}
	return value
}

func (s *notificationService) databaseUnread(ctx context.Context, userID uuid.UUID) int64 {
	count, err := s.notifications.CountUnread(ctx, userID)
	if err != nil {
		s.logf("failed to read unread count from database for %s: %v", userID, err)
		return 0
	}
	if s.cache != nil {
		_ = s.cache.SetUnread(ctx, userID, int64(count))
	}
	return int64(count)
}

// -----------------------------------------------------------------------------
// Inbox mutations
// -----------------------------------------------------------------------------

func (s *notificationService) MarkAsRead(ctx context.Context, userID string, ids []string) (int32, error) {
	recipientID, err := parseUserID(userID)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, fmt.Errorf("%w: at least one notification id is required", models.ErrInvalidInput)
	}

	parsed := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, raw := range ids {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return 0, fmt.Errorf("%w: invalid notification id %q", models.ErrInvalidInput, raw)
		}
		key := id.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		parsed = append(parsed, key)
	}

	changed, err := s.notifications.MarkRead(ctx, recipientID, parsed)
	if err != nil {
		return 0, err
	}

	if changed == 0 {
		return s.unreadCount(ctx, recipientID), nil
	}

	if s.cache != nil {
		remaining, err := s.cache.DecrUnread(ctx, recipientID, changed)
		if err == nil {
			return clampInt32(remaining), nil
		}
		s.logf("failed to decrement unread counter for %s: %v", recipientID, err)
	}

	return s.unreadCount(ctx, recipientID), nil
}

func (s *notificationService) MarkAllAsRead(ctx context.Context, userID string) (int32, error) {
	recipientID, err := parseUserID(userID)
	if err != nil {
		return 0, err
	}

	changed, err := s.notifications.MarkAllRead(ctx, recipientID)
	if err != nil {
		return 0, err
	}

	if s.cache != nil {
		if err := s.cache.ResetUnread(ctx, recipientID); err != nil {
			s.logf("failed to reset unread counter for %s: %v", recipientID, err)
		}
	}

	return clampInt32(int64(changed)), nil
}

func (s *notificationService) DeleteNotification(ctx context.Context, userID, notificationID string) error {
	recipientID, err := parseUserID(userID)
	if err != nil {
		return err
	}

	id, err := uuid.Parse(strings.TrimSpace(notificationID))
	if err != nil {
		return fmt.Errorf("%w: invalid notification id", models.ErrInvalidInput)
	}

	wasUnread, err := s.notifications.Delete(ctx, recipientID, id)
	if err != nil {
		return err
	}

	if wasUnread && s.cache != nil {
		if _, err := s.cache.DecrUnread(ctx, recipientID, 1); err != nil {
			s.logf("failed to decrement badge after deleting notification %s: %v", id, err)
		}
	}
	return nil
}

// -----------------------------------------------------------------------------
// Preferences and devices
// -----------------------------------------------------------------------------

func (s *notificationService) GetSettings(ctx context.Context, userID string) (*models.NotificationSettings, error) {
	recipientID, err := parseUserID(userID)
	if err != nil {
		return nil, err
	}
	return s.settings.Get(ctx, recipientID)
}

func (s *notificationService) UpdateSettings(ctx context.Context, userID string, settings *models.NotificationSettings) (*models.NotificationSettings, error) {
	recipientID, err := parseUserID(userID)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return nil, fmt.Errorf("%w: settings payload is required", models.ErrInvalidInput)
	}

	// The path user_id is authoritative; never let a body overwrite ownership.
	settings.UserID = recipientID
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	if err := s.settings.Save(ctx, settings); err != nil {
		return nil, err
	}
	return settings, nil
}

func (s *notificationService) RegisterDeviceToken(ctx context.Context, userID, token, platform, deviceName, appVersion string) error {
	recipientID, err := parseUserID(userID)
	if err != nil {
		return err
	}
	if err := models.ValidateDeviceToken(recipientID, token, platform); err != nil {
		return err
	}
	return s.devices.Register(ctx, recipientID, strings.TrimSpace(token), strings.ToLower(strings.TrimSpace(platform)), deviceName, appVersion)
}

func (s *notificationService) RevokeDeviceToken(ctx context.Context, userID, token string) error {
	recipientID, err := parseUserID(userID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("%w: token is required", models.ErrInvalidInput)
	}
	return s.devices.Revoke(ctx, recipientID, strings.TrimSpace(token))
}

// -----------------------------------------------------------------------------
// Event write path
// -----------------------------------------------------------------------------

func (s *notificationService) ProcessEvent(ctx context.Context, request *models.NotificationRequest) error {
	if request == nil {
		return fmt.Errorf("%w: notification request is required", models.ErrInvalidInput)
	}

	// Self-actions are dropped silently: liking or commenting on your own post
	// must never notify you.
	if models.IsSelfAction(request.ActorID, request.RecipientID) {
		return nil
	}

	if err := request.Validate(); err != nil {
		return err
	}

	if !request.BypassPolicy {
		decision := s.policy.CheckInApp(ctx, request.RecipientID, request.Type)
		if !decision.Allowed {
			s.logf("dropping %s notification for %s: %s", request.Type, request.RecipientID, decision.Reason)
			return nil
		}
	}

	dedupKey := ""
	dedupAcquired := false
	if s.shouldDedup(request) && s.cache != nil {
		dedupKey = request.DedupKey()
		acquired, err := s.cache.AcquireDedup(ctx, dedupKey, s.options.DedupTTL)
		if err != nil {
			// Redis being down disables dedup, not ingestion.
			s.logf("dedup unavailable for %s: %v", request.Type, err)
		} else if !acquired {
			return nil
		} else {
			dedupAcquired = true
		}
	}

	err := s.process(ctx, request)
	if err != nil && dedupAcquired {
		// Let the broker retry immediately instead of waiting out the TTL.
		_ = s.cache.ReleaseDedup(ctx, dedupKey)
	}
	return err
}

func (s *notificationService) shouldDedup(request *models.NotificationRequest) bool {
	if request.EventID != "" {
		return true
	}
	// Alerts without a producer-supplied event ID intentionally skip dedup:
	// two genuine security alerts within 60 seconds must both be delivered.
	if request.Type == models.TypeSystemAlert && request.ActorID == nil {
		return false
	}
	return true
}

func (s *notificationService) process(ctx context.Context, request *models.NotificationRequest) error {
	actor := s.resolveActor(ctx, request)

	if request.Type.Aggregatable() && request.ActorID != nil && s.aggregator != nil {
		return s.processAggregated(ctx, request, actor)
	}
	return s.createNotification(ctx, request, actor, "")
}

func (s *notificationService) createNotification(ctx context.Context, request *models.NotificationRequest, actor models.Actor, aggregationKey string) error {
	actorIDs := []string{}
	hints := map[string]models.Actor{}
	if request.ActorID != nil {
		actorID := request.ActorID.String()
		actorIDs = append(actorIDs, actorID)
		hints[actorID] = actor
	}

	actors := s.actorsForIDs(ctx, actorIDs, hints)

	entityType := request.EntityType
	if entityType == "" {
		entityType = request.Type.EntityTypeFor()
	}

	entityID := request.EntityID
	if entityID == "" {
		switch {
		case request.Type == models.TypeFollow && request.ActorID != nil:
			entityID = request.ActorID.String()
		case request.Type == models.TypeSystemAlert:
			entityID = "system"
		}
	}

	title := strings.TrimSpace(request.TitleOverride)
	if title == "" {
		if request.Type == models.TypeSystemAlert {
			title = "Security alert"
		} else {
			title = models.RenderTitle(request.Type, actors, 1)
		}
	}

	preview := strings.TrimSpace(request.BodyOverride)
	if preview == "" {
		preview = models.RenderPreview(request.Type, request.ContentPreview)
	}

	actionURL := strings.TrimSpace(request.ActionURL)
	if actionURL == "" {
		actorID := ""
		if request.ActorID != nil {
			actorID = request.ActorID.String()
		}
		actionURL = models.DefaultActionURL(request.Type, entityID, actorID)
	}

	created, err := s.notifications.Create(ctx, &models.CreateNotificationInput{
		RecipientID:      request.RecipientID,
		Type:             request.Type,
		EntityID:         entityID,
		EntityType:       entityType,
		ActorIDs:         actorIDs,
		TotalActorsCount: 1,
		Title:            title,
		ContentPreview:   preview,
		ActionURL:        actionURL,
	})
	if err != nil {
		return err
	}
	created.Actors = actors

	unread := s.unreadAfterCreate(ctx, request.RecipientID)

	if aggregationKey != "" && s.aggregator != nil && request.ActorID != nil {
		if err := s.aggregator.Remember(ctx, aggregationKey, created.ID.String()); err != nil {
			s.logf("failed to remember aggregation window for %s: %v", created.ID, err)
		}
		if _, err := s.aggregator.AddActor(ctx, aggregationKey, request.ActorID.String()); err != nil {
			s.logf("failed to seed aggregation actor set for %s: %v", created.ID, err)
		}
	}

	s.publishRealtime(ctx, created, unread)
	s.enqueuePush(ctx, created)
	s.enqueueEmail(ctx, created)
	return nil
}

func (s *notificationService) processAggregated(ctx context.Context, request *models.NotificationRequest, actor models.Actor) error {
	aggregationKey := s.aggregator.Key(request.RecipientID, request.Type, request.EntityID)

	existingID, found, err := s.aggregator.Existing(ctx, aggregationKey)
	if err != nil || !found {
		return s.createNotification(ctx, request, actor, aggregationKey)
	}

	notificationID, err := uuid.Parse(existingID)
	if err != nil {
		_ = s.aggregator.Forget(ctx, aggregationKey)
		return s.createNotification(ctx, request, actor, aggregationKey)
	}

	actorID := request.ActorID.String()
	added, err := s.aggregator.AddActor(ctx, aggregationKey, actorID)
	if err == nil && !added {
		// The same actor already appears in this window.
		return nil
	}

	notification, err := s.notifications.GetByID(ctx, notificationID)
	if err != nil {
		_ = s.aggregator.Forget(ctx, aggregationKey)
		if errors.Is(err, models.ErrNotificationNotFound) {
			return s.createNotification(ctx, request, actor, aggregationKey)
		}
		return err
	}

	actorIDs := appendActorID(notification.ActorIDs, actorID, 3)
	hints := map[string]models.Actor{actorID: actor}
	actors := s.actorsForIDs(ctx, actorIDs, hints)

	total := notification.TotalActorsCount + 1
	title := models.RenderTitle(request.Type, actors, total)
	preview := notification.ContentPreview
	if preview == "" {
		preview = models.RenderPreview(request.Type, request.ContentPreview)
	}

	wasRead, err := s.notifications.UpdateAggregate(ctx, notification.ID, actorIDs, total, title, preview, true)
	if err != nil {
		if errors.Is(err, models.ErrNotificationNotFound) {
			_ = s.aggregator.Forget(ctx, aggregationKey)
			return s.createNotification(ctx, request, actor, aggregationKey)
		}
		return err
	}

	notification.ActorIDs = actorIDs
	notification.Actors = actors
	notification.TotalActorsCount = total
	notification.Title = title
	notification.ContentPreview = preview
	notification.IsRead = false

	unread := s.unreadAfterAggregate(ctx, request.RecipientID, wasRead)

	// Aggregated roll-ups suppress push and throttle realtime frames: one
	// "Alice and 45 others liked your post" toast is enough.
	if s.aggregator.ShouldBroadcast(ctx, aggregationKey) {
		s.publishRealtime(ctx, notification, unread)
	}
	return nil
}

// RevokeAggregatedActor implements the "un-like" correction path described in
// SYSTEM_DESIGN.md: decrement the roll-up, and delete it once no actors remain.
func (s *notificationService) RevokeAggregatedActor(ctx context.Context, request *models.NotificationRequest) error {
	if request == nil || request.ActorID == nil || s.aggregator == nil {
		return nil
	}
	if models.IsSelfAction(request.ActorID, request.RecipientID) {
		return nil
	}
	if !request.Type.Aggregatable() || request.EntityID == "" {
		return nil
	}

	aggregationKey := s.aggregator.Key(request.RecipientID, request.Type, request.EntityID)
	existingID, found, err := s.aggregator.Existing(ctx, aggregationKey)
	if err != nil || !found {
		return nil
	}

	actorID := request.ActorID.String()
	removed, err := s.aggregator.RemoveActor(ctx, aggregationKey, actorID)
	if err != nil || !removed {
		return nil
	}

	notificationID, err := uuid.Parse(existingID)
	if err != nil {
		return nil
	}
	notification, err := s.notifications.GetByID(ctx, notificationID)
	if err != nil {
		if errors.Is(err, models.ErrNotificationNotFound) {
			_ = s.aggregator.Forget(ctx, aggregationKey)
			return nil
		}
		return err
	}

	total := notification.TotalActorsCount - 1
	if total <= 0 {
		wasUnread, err := s.notifications.Delete(ctx, request.RecipientID, notification.ID)
		if err != nil && !errors.Is(err, models.ErrNotificationNotFound) {
			return err
		}
		if wasUnread && s.cache != nil {
			if _, err := s.cache.DecrUnread(ctx, request.RecipientID, 1); err != nil {
				s.logf("failed to decrement badge after revoking notification %s: %v", notification.ID, err)
			}
		}
		_ = s.aggregator.Forget(ctx, aggregationKey)
		return nil
	}

	actorIDs := removeActorID(notification.ActorIDs, actorID)
	hint := models.Actor{
		ID:          actorID,
		Username:    request.ActorUsername,
		DisplayName: request.ActorDisplayName,
		AvatarURL:   request.ActorAvatarURL,
	}
	actors := s.actorsForIDs(ctx, actorIDs, map[string]models.Actor{actorID: hint})
	title := models.RenderTitle(notification.Type, actors, total)

	// reopen=false: a retracted like must not resurrect a notification the user
	// already read.
	if _, err := s.notifications.UpdateAggregate(ctx, notification.ID, actorIDs, total, title, notification.ContentPreview, false); err != nil {
		if errors.Is(err, models.ErrNotificationNotFound) {
			_ = s.aggregator.Forget(ctx, aggregationKey)
			return nil
		}
		return err
	}
	return nil
}

// -----------------------------------------------------------------------------
// Dispatch helpers
// -----------------------------------------------------------------------------

func (s *notificationService) publishRealtime(ctx context.Context, notification *models.Notification, unread int64) {
	if s.realtime == nil {
		return
	}
	payload := models.NewRealtimePayload(notification, unread)
	if err := s.realtime.Publish(ctx, notification.RecipientID.String(), payload); err != nil {
		s.logf("failed to publish realtime notification %s: %v", notification.ID, err)
	}
}

func (s *notificationService) enqueuePush(ctx context.Context, notification *models.Notification) {
	if s.push == nil {
		return
	}

	decision := s.policy.CheckPush(ctx, notification.RecipientID, notification.Type)
	if !decision.Allowed {
		s.logf("suppressing push for %s: %s", notification.ID, decision.Reason)
		return
	}

	body := notification.ContentPreview
	if body == "" {
		body = notification.Title
	}

	message := models.PushMessage{
		RecipientID:     notification.RecipientID,
		NotificationID:  notification.ID.String(),
		Title:           notification.Title,
		Body:            body,
		ActionURL:       notification.ActionURL,
		IsSecurityAlert: notification.Type == models.TypeSystemAlert,
	}
	if err := s.push.Enqueue(ctx, message); err != nil {
		s.logf("failed to enqueue push for %s: %v", notification.ID, err)
	}
}

func (s *notificationService) enqueueEmail(ctx context.Context, notification *models.Notification) {
	if s.email == nil || notification.Type != models.TypeSystemAlert {
		return
	}

	decision := s.policy.CheckEmail(ctx, notification.RecipientID, notification.Type)
	if !decision.Allowed {
		s.logf("suppressing email for %s: %s", notification.ID, decision.Reason)
		return
	}

	email := ""
	if s.actors != nil {
		people := s.actors.GetActors(ctx, []string{notification.RecipientID.String()})
		if actor, ok := people[notification.RecipientID.String()]; ok {
			email = actor.Email
		}
	}
	if email == "" {
		s.logf("skipping email for %s: recipient has no email address", notification.ID)
		return
	}

	body := notification.Title
	if notification.ContentPreview != "" {
		body += "\n\n" + notification.ContentPreview
	}
	if notification.ActionURL != "" {
		body += "\n\n" + notification.ActionURL
	}

	message := models.EmailMessage{
		RecipientID:    notification.RecipientID,
		RecipientEmail: email,
		Subject:        notification.Title,
		Body:           body,
		NotificationID: notification.ID.String(),
	}
	if err := s.email.Enqueue(ctx, message); err != nil {
		s.logf("failed to enqueue email for %s: %v", notification.ID, err)
	}
}

// -----------------------------------------------------------------------------
// Actor hydration
// -----------------------------------------------------------------------------

func (s *notificationService) resolveActor(ctx context.Context, request *models.NotificationRequest) models.Actor {
	if request.ActorID == nil {
		return models.Actor{}
	}

	hint := models.Actor{
		ID:          request.ActorID.String(),
		Username:    request.ActorUsername,
		DisplayName: request.ActorDisplayName,
		AvatarURL:   request.ActorAvatarURL,
	}

	if s.actors == nil {
		return hint
	}

	fetched, ok := s.actors.GetActors(ctx, []string{hint.ID})[hint.ID]
	if !ok {
		return hint
	}
	return mergeActor(fetched, hint)
}

func (s *notificationService) actorsForIDs(ctx context.Context, ids []string, hints map[string]models.Actor) []models.Actor {
	actors := make([]models.Actor, 0, len(ids))
	if len(ids) == 0 {
		return actors
	}

	resolved := map[string]models.Actor{}
	if s.actors != nil {
		resolved = s.actors.GetActors(ctx, ids)
	}

	for _, id := range ids {
		actor, ok := resolved[id]
		if !ok {
			actor = hints[id]
		}
		if actor.ID == "" {
			actor.ID = id
		}
		actors = append(actors, actor)
	}
	return actors
}

func (s *notificationService) hydrateNotifications(ctx context.Context, notifications []models.Notification) {
	if len(notifications) == 0 {
		return
	}

	ids := make([]string, 0, len(notifications)*3)
	for _, notification := range notifications {
		ids = append(ids, notification.ActorIDs...)
	}

	resolved := map[string]models.Actor{}
	if s.actors != nil && len(ids) > 0 {
		resolved = s.actors.GetActors(ctx, ids)
	}

	for i := range notifications {
		notification := &notifications[i]
		actors := make([]models.Actor, 0, len(notification.ActorIDs))
		for _, id := range notification.ActorIDs {
			actor, ok := resolved[id]
			if !ok {
				actor = models.Actor{ID: id}
			}
			actors = append(actors, actor)
		}
		notification.Actors = actors

		if notification.Type != models.TypeSystemAlert {
			notification.Title = models.RenderTitle(notification.Type, actors, notification.TotalActorsCount)
		}
	}
}

func mergeActor(fetched, hint models.Actor) models.Actor {
	if fetched.ID == "" {
		fetched.ID = hint.ID
	}
	if fetched.Username == "" {
		fetched.Username = hint.Username
	}
	if fetched.DisplayName == "" {
		fetched.DisplayName = hint.DisplayName
	}
	if fetched.AvatarURL == "" {
		fetched.AvatarURL = hint.AvatarURL
	}
	if fetched.Email == "" {
		fetched.Email = hint.Email
	}
	return fetched
}

// -----------------------------------------------------------------------------
// Small helpers
// -----------------------------------------------------------------------------

func parseUserID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("%w: invalid user_id", models.ErrInvalidInput)
	}
	return id, nil
}

func appendActorID(ids []string, id string, max int) []string {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	if len(ids) >= max {
		return ids
	}
	return append(ids, id)
}

func removeActorID(ids []string, id string) []string {
	result := make([]string, 0, len(ids))
	for _, existing := range ids {
		if existing != id {
			result = append(result, existing)
		}
	}
	return result
}

func clampInt32(value int64) int32 {
	const maxInt32 = int64(1<<31 - 1)
	if value < 0 {
		return 0
	}
	if value > maxInt32 {
		return int32(maxInt32)
	}
	return int32(value)
}

func (s *notificationService) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
}
