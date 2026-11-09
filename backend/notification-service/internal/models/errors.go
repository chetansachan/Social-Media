package models

import "errors"

var (
	// ErrInvalidInput is returned when a request or event fails validation.
	ErrInvalidInput = errors.New("invalid input")

	// ErrInvalidCursor is returned when a pagination cursor cannot be decoded.
	ErrInvalidCursor = errors.New("invalid cursor")

	// ErrNotificationNotFound is returned when a notification does not exist,
	// belongs to another recipient, or has been deleted.
	ErrNotificationNotFound = errors.New("notification not found")

	// ErrPermanentEvent marks an event that can never succeed on retry. The
	// consumer acknowledges/DLQs these instead of leaving them pending.
	ErrPermanentEvent = errors.New("permanent event error")

	// ErrCacheUnavailable means Redis is not configured or is unreachable.
	ErrCacheUnavailable = errors.New("cache unavailable")

	// ErrTokenUnregistered is returned by push providers when APNs/FCM reports
	// that a device token is no longer valid.
	ErrTokenUnregistered = errors.New("push token unregistered")
)
