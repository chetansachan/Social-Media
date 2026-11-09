package models

import "errors"

var (
	ErrInvalidInput     = errors.New("invalid input")
	ErrInvalidCursor    = errors.New("invalid pagination cursor")
	ErrFeedUnavailable  = errors.New("feed temporarily unavailable")
	ErrCacheUnavailable = errors.New("timeline cache unavailable")
)
