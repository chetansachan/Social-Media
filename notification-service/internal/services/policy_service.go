package services

import (
	"context"
	"fmt"
	"time"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/google/uuid"
)

// Decision is the outcome of evaluating one channel for one notification.
type Decision struct {
	Allowed bool
	Reason  string
}

// PolicyService evaluates category toggles, channel master switches, quiet
// hours, and the per-user push budget.
type PolicyService struct {
	settings   SettingsStore
	cache      CacheStore
	maxPush    int
	pushWindow time.Duration
	now        func() time.Time
}

func NewPolicyService(settings SettingsStore, cache CacheStore, options Options) *PolicyService {
	options = options.withDefaults()
	return &PolicyService{
		settings:   settings,
		cache:      cache,
		maxPush:    options.MaxPushPerHour,
		pushWindow: options.PushRateWindow,
		now:        options.Now,
	}
}

// CheckInApp fails open on a settings-store outage: hiding a user's inbox
// because PostgreSQL is briefly unavailable is worse than briefly honouring a
// muted category.
func (p *PolicyService) CheckInApp(ctx context.Context, userID uuid.UUID, notificationType models.NotificationType) Decision {
	if notificationType == models.TypeSystemAlert {
		return Decision{Allowed: true, Reason: "system_alert"}
	}
	if p == nil || p.settings == nil {
		return Decision{Allowed: true, Reason: "settings_unavailable"}
	}

	settings, err := p.settings.Get(ctx, userID)
	if err != nil {
		return Decision{Allowed: true, Reason: "settings_unavailable"}
	}
	if !settings.InAppEnabled(notificationType) {
		return Decision{Allowed: false, Reason: "in_app_category_disabled"}
	}
	return Decision{Allowed: true, Reason: "in_app_enabled"}
}

// CheckPush fails closed on a settings-store outage to avoid alert storms.
// Security alerts bypass category toggles, quiet hours, and rate limiting.
func (p *PolicyService) CheckPush(ctx context.Context, userID uuid.UUID, notificationType models.NotificationType) Decision {
	if notificationType == models.TypeSystemAlert {
		return Decision{Allowed: true, Reason: "system_alert"}
	}
	if p == nil || p.settings == nil {
		return Decision{Allowed: false, Reason: "settings_unavailable"}
	}

	settings, err := p.settings.Get(ctx, userID)
	if err != nil {
		return Decision{Allowed: false, Reason: "settings_unavailable"}
	}
	if !settings.PushCategoryEnabled(notificationType) {
		return Decision{Allowed: false, Reason: "push_category_disabled"}
	}
	if settings.IsQuietHours(p.now()) {
		return Decision{Allowed: false, Reason: "quiet_hours"}
	}

	if p.cache != nil {
		allowed, err := p.cache.AllowPush(ctx, userID, p.maxPush, p.pushWindow)
		if err != nil {
			// The rate limiter is a spam control, not a correctness boundary.
			// Allow the push and let the worker/provider absorb the load.
			return Decision{Allowed: true, Reason: "rate_limit_unavailable"}
		}
		if !allowed {
			return Decision{Allowed: false, Reason: "push_rate_limited"}
		}
	}
	return Decision{Allowed: true, Reason: "push_enabled"}
}

// CheckEmail currently gates security alerts. Digest batching is handled by the
// email dispatcher/provider.
func (p *PolicyService) CheckEmail(ctx context.Context, userID uuid.UUID, notificationType models.NotificationType) Decision {
	if p == nil || p.settings == nil {
		return Decision{Allowed: false, Reason: "settings_unavailable"}
	}
	settings, err := p.settings.Get(ctx, userID)
	if err != nil {
		return Decision{Allowed: false, Reason: "settings_unavailable"}
	}
	if !settings.EmailCategoryEnabled(notificationType) {
		return Decision{Allowed: false, Reason: "email_disabled"}
	}
	return Decision{Allowed: true, Reason: "email_enabled"}
}

func (p *PolicyService) String() string {
	return fmt.Sprintf("policy(push=%d/%s)", p.maxPush, p.pushWindow)
}
