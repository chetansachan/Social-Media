package models

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Report target types stored in content_reports.target_type.
const (
	TargetPost    = "POST"
	TargetComment = "COMMENT"
	TargetUser    = "USER"
)

// Report reasons stored in content_reports.reason.
const (
	ReasonSpam          = "SPAM"
	ReasonHarassment    = "HARASSMENT"
	ReasonHate          = "HATE"
	ReasonViolence      = "VIOLENCE"
	ReasonNSFW          = "NSFW"
	ReasonImpersonation = "IMPERSONATION"
	ReasonSelfHarm      = "SELF_HARM"
	ReasonOther         = "OTHER"
)

// Report statuses stored in content_reports.status.
const (
	StatusPending   = "PENDING"
	StatusReviewing = "REVIEWING"
	StatusActioned  = "ACTIONED"
	StatusDismissed = "DISMISSED"
)

// Report is one piece of flagged content waiting for review.
type Report struct {
	ID         uuid.UUID  `json:"id"`
	ReporterID uuid.UUID  `json:"reporter_id"`
	TargetType string     `json:"target_type"`
	TargetID   uuid.UUID  `json:"target_id"`
	Reason     string     `json:"reason"`
	Details    string     `json:"details"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	ReviewerID *uuid.UUID `json:"reviewer_id,omitempty"`
	ReviewNote string     `json:"review_note,omitempty"`
}

// Visibility is the caller's moderation view of another user.
type Visibility struct {
	// IsBlocked means the viewer blocked the target.
	IsBlocked bool
	// HasBlockedViewer means the target blocked the viewer.
	HasBlockedViewer bool
	IsMuted          bool
}

// IsHidden reports whether either side blocked the other. Blocking is
// symmetric in effect even though only one row exists.
func (v Visibility) IsHidden() bool {
	return v.IsBlocked || v.HasBlockedViewer
}

// IsTargetType reports whether value is a supported report target.
func IsTargetType(value string) bool {
	switch value {
	case TargetPost, TargetComment, TargetUser:
		return true
	default:
		return false
	}
}

// IsReason reports whether value is a supported report reason.
func IsReason(value string) bool {
	switch value {
	case ReasonSpam, ReasonHarassment, ReasonHate, ReasonViolence,
		ReasonNSFW, ReasonImpersonation, ReasonSelfHarm, ReasonOther:
		return true
	default:
		return false
	}
}

// IsOpenStatus reports whether a report has not been resolved yet.
func IsOpenStatus(value string) bool {
	return value == StatusPending || value == StatusReviewing
}

// IsResolvableStatus reports whether a reviewer may move a report to value.
func IsResolvableStatus(value string) bool {
	return value == StatusReviewing || value == StatusActioned || value == StatusDismissed
}

// ReportPage is a keyset-paginated slice of the review queue.
type ReportPage struct {
	Reports    []Report
	NextCursor string
	HasMore    bool
}

// ReportCursor is the keyset position of the last report in a page. Reports
// page newest first with the UUID as a tie-breaker.
type ReportCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
} // IsZero reports whether the cursor is unset (first page).
func (c ReportCursor) IsZero() bool {
	return c.ID == uuid.Nil && c.CreatedAt.IsZero()
}

// EncodeReportCursor serialises a keyset position as a URL-safe opaque token.
func EncodeReportCursor(createdAt time.Time, id uuid.UUID) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeReportCursor parses a cursor produced by EncodeReportCursor.
func DecodeReportCursor(encoded string) (ReportCursor, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return ReportCursor{}, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return ReportCursor{}, fmt.Errorf("%w: malformed report cursor", ErrInvalidInput)
	}

	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return ReportCursor{}, fmt.Errorf("%w: report cursor has no position", ErrInvalidInput)
	}

	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return ReportCursor{}, fmt.Errorf("%w: report cursor has an invalid timestamp", ErrInvalidInput)
	}

	id, err := uuid.Parse(parts[1])
	if err != nil {
		return ReportCursor{}, fmt.Errorf("%w: report cursor has an invalid id", ErrInvalidInput)
	}

	return ReportCursor{CreatedAt: createdAt, ID: id}, nil
}
