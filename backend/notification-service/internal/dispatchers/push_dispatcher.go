package dispatchers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/google/uuid"
)

// ErrQueueFull is returned when the bounded worker queue cannot accept more
// work. The caller logs and moves on; the notification is already durable.
var ErrQueueFull = errors.New("dispatcher queue is full")

// DeviceTokenLister is the read side of user_device_tokens needed by push.
type DeviceTokenLister interface {
	ListActive(ctx context.Context, userID uuid.UUID) ([]models.DeviceToken, error)
	MarkInactive(ctx context.Context, token string) error
}

// PushProvider is implemented by FCM, APNs, and the development log provider.
type PushProvider interface {
	Send(ctx context.Context, device models.DeviceToken, message models.PushMessage) error
}

// PushDispatcher is a bounded worker pool that fans one notification out to all
// of a user's active devices. A slow provider can never block the stream
// consumer: Enqueue either succeeds immediately or drops with ErrQueueFull.
type PushDispatcher struct {
	devices  DeviceTokenLister
	provider PushProvider
	queue    chan models.PushMessage
	workers  int
	logger   *log.Logger
	metrics  DispatchRecorder
	wg       sync.WaitGroup
}

func NewPushDispatcher(devices DeviceTokenLister, provider PushProvider, workers, queueSize int, logger *log.Logger) *PushDispatcher {
	if workers <= 0 {
		workers = 8
	}
	if queueSize <= 0 {
		queueSize = 1024
	}
	if logger == nil {
		logger = log.Default()
	}
	return &PushDispatcher{
		devices:  devices,
		provider: provider,
		queue:    make(chan models.PushMessage, queueSize),
		workers:  workers,
		logger:   logger,
	}
}

// SetMetrics wires delivery counters into the dispatcher. It is safe to call
// with nil to disable them.
func (d *PushDispatcher) SetMetrics(recorder DispatchRecorder) {
	if d == nil {
		return
	}
	d.metrics = recorder
}

func (d *PushDispatcher) Start(ctx context.Context) {
	for i := 0; i < d.workers; i++ {
		d.wg.Add(1)
		go d.worker(ctx, i)
	}
}

func (d *PushDispatcher) Wait() {
	d.wg.Wait()
}

func (d *PushDispatcher) Enqueue(ctx context.Context, message models.PushMessage) error {
	if d == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case d.queue <- message:
		return nil
	default:
		recordDispatch(d.metrics, "push", "queue_full")
		return ErrQueueFull
	}
}

func (d *PushDispatcher) worker(ctx context.Context, workerID int) {
	defer d.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case message := <-d.queue:
			d.deliver(ctx, message)
		}
	}
}

func (d *PushDispatcher) deliver(ctx context.Context, message models.PushMessage) {
	if d.devices == nil || d.provider == nil {
		return
	}

	tokens, err := d.devices.ListActive(ctx, message.RecipientID)
	if err != nil {
		recordDispatch(d.metrics, "push", "error")
		d.logger.Printf("push: failed to list devices for %s: %v", message.RecipientID, err)
		return
	}
	if len(tokens) == 0 {
		recordDispatch(d.metrics, "push", "no_devices")
		return
	}

	for _, device := range tokens {
		if ctx.Err() != nil {
			return
		}

		err := retry(ctx, 3, func() error {
			return d.provider.Send(ctx, device, message)
		})
		if err == nil {
			recordDispatch(d.metrics, "push", "success")
			continue
		}

		if errors.Is(err, models.ErrTokenUnregistered) {
			recordDispatch(d.metrics, "push", "token_unregistered")
			if markErr := d.devices.MarkInactive(ctx, device.Token); markErr != nil {
				d.logger.Printf("push: failed to deactivate stale token for user %s: %v", message.RecipientID, markErr)
			}
			continue
		}

		recordDispatch(d.metrics, "push", "error")
		d.logger.Printf("push: delivery failed for user %s on %s: %v", message.RecipientID, device.Platform, err)
	}
}

func retry(ctx context.Context, attempts int, operation func() error) error {
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	backoff := 100 * time.Millisecond
	for attempt := 0; attempt < attempts; attempt++ {
		if err := operation(); err != nil {
			lastErr = err
			if errors.Is(err, models.ErrTokenUnregistered) {
				return err
			}
		} else {
			return nil
		}

		if attempt == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return lastErr
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-3]) + "..."
}

// stringID is a small guard used by provider payloads where a UUID must never
// be empty in the deep link metadata.
func stringID(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func providerError(provider string, status int, body []byte) error {
	return fmt.Errorf("%s returned status %d: %s", provider, status, truncate(string(body), 300))
}
