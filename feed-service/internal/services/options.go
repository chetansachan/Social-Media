package services

import "time"

// Options contains the feed policy knobs shared by the read and write paths.
type Options struct {
	CelebrityThreshold   int32
	TimelineMaxSize      int64
	TimelineTTL          time.Duration
	ActiveUserTTL        time.Duration
	CelebrityRecentLimit int64
	DefaultPageSize      int32
	MaxPageSize          int32
}

func (o Options) withDefaults() Options {
	if o.CelebrityThreshold <= 0 {
		o.CelebrityThreshold = 25000
	}
	if o.TimelineMaxSize <= 0 {
		o.TimelineMaxSize = 800
	}
	if o.TimelineTTL <= 0 {
		o.TimelineTTL = 720 * time.Hour
	}
	if o.ActiveUserTTL <= 0 {
		o.ActiveUserTTL = 14 * 24 * time.Hour
	}
	if o.CelebrityRecentLimit <= 0 {
		o.CelebrityRecentLimit = 20
	}
	if o.DefaultPageSize <= 0 {
		o.DefaultPageSize = 20
	}
	if o.MaxPageSize <= 0 {
		o.MaxPageSize = 100
	}
	if o.DefaultPageSize > o.MaxPageSize {
		o.DefaultPageSize = o.MaxPageSize
	}
	return o
}
