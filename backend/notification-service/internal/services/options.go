package services

import "time"

// Options holds the behavioural knobs that are not repository concerns.
type Options struct {
	AggregationWindow time.Duration
	DedupTTL          time.Duration
	RealtimeThrottle  time.Duration
	MaxPushPerHour    int
	PushRateWindow    time.Duration

	// Now is injectable so policy and dedup tests do not depend on wall time.
	Now func() time.Time
}

// DefaultOptions matches the production defaults in config.Load.
func DefaultOptions() Options {
	return Options{
		AggregationWindow: 15 * time.Minute,
		DedupTTL:          60 * time.Second,
		RealtimeThrottle:  30 * time.Second,
		MaxPushPerHour:    5,
		PushRateWindow:    time.Hour,
		Now:               time.Now,
	}
}

func (o Options) withDefaults() Options {
	defaults := DefaultOptions()
	if o.AggregationWindow <= 0 {
		o.AggregationWindow = defaults.AggregationWindow
	}
	if o.DedupTTL <= 0 {
		o.DedupTTL = defaults.DedupTTL
	}
	if o.RealtimeThrottle <= 0 {
		o.RealtimeThrottle = defaults.RealtimeThrottle
	}
	if o.MaxPushPerHour <= 0 {
		o.MaxPushPerHour = defaults.MaxPushPerHour
	}
	if o.PushRateWindow <= 0 {
		o.PushRateWindow = defaults.PushRateWindow
	}
	if o.Now == nil {
		o.Now = defaults.Now
	}
	return o
}
