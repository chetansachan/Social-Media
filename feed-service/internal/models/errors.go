package models

import "errors"

var (
	ErrInvalidInput      = errors.New("invalid input")
	ErrFeedUnavailable   = errors.New("feed unavailable")
	ErrUserNotFound      = errors.New("user not found")
	ErrInvalidCursor     = errors.New("invalid pagination cursor")
	ErrCacheUnavailable  = errors.New("timeline cache unavailable")
)
