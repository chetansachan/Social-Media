package models

import "errors"

var (
	ErrInvalidInput = errors.New("invalid input")
	// ErrCannotModerateSelf stops a user from blocking, muting, or reporting
	// themselves; a self-block would make their own feed disappear.
	ErrCannotModerateSelf = errors.New("cannot moderate yourself")
	ErrReportNotFound     = errors.New("report not found")
	// ErrReportClosed is returned when a reviewer tries to resolve a report
	// that somebody already actioned or dismissed.
	ErrReportClosed = errors.New("report already resolved")
)
