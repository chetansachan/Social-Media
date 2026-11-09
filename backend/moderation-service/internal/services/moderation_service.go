package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/chetansachan/socialflow/moderation-service/internal/models"
	"github.com/chetansachan/socialflow/moderation-service/internal/repository"
	"github.com/google/uuid"
)

// ModerationService validates moderation requests and keeps the repository
// free of caller-facing rules (self-targeting, enum membership, page limits).
type ModerationService interface {
	BlockUser(ctx context.Context, blockerID, blockedID string) error
	UnblockUser(ctx context.Context, blockerID, blockedID string) error
	MuteUser(ctx context.Context, muterID, mutedID string) error
	UnmuteUser(ctx context.Context, muterID, mutedID string) error
	GetStatus(ctx context.Context, viewerID, targetID string) (models.Visibility, error)
	ListBlocked(ctx context.Context, userID string) ([]string, error)
	ListMuted(ctx context.Context, userID string) ([]string, error)
	ReportContent(ctx context.Context, reporterID, targetType, targetID, reason, details string) (*models.Report, bool, error)
	ListReports(ctx context.Context, status string, limit int32, cursor string) (*models.ReportPage, error)
	ResolveReport(ctx context.Context, reportID, reviewerID, status, note string) (*models.Report, error)
}

const (
	maxReportDetails = 2000
	defaultPageSize  = 20
	maxPageSize      = 100
)

type moderationService struct {
	repo repository.ModerationRepository
}

func NewModerationService(repo repository.ModerationRepository) ModerationService {
	return &moderationService{repo: repo}
}

// parseRelationship validates both sides of a block or mute and rejects
// self-targeting, which would otherwise hide a user's own content.
func parseRelationship(firstID, secondID, firstLabel, secondLabel string) (uuid.UUID, uuid.UUID, error) {
	first, err := uuid.Parse(strings.TrimSpace(firstID))
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("%w: invalid %s", models.ErrInvalidInput, firstLabel)
	}
	second, err := uuid.Parse(strings.TrimSpace(secondID))
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("%w: invalid %s", models.ErrInvalidInput, secondLabel)
	}
	if first == second {
		return uuid.Nil, uuid.Nil, models.ErrCannotModerateSelf
	}
	return first, second, nil
}

func (s *moderationService) BlockUser(ctx context.Context, blockerID, blockedID string) error {
	blocker, blocked, err := parseRelationship(blockerID, blockedID, "blocker id", "blocked id")
	if err != nil {
		return err
	}
	return s.repo.Block(ctx, blocker, blocked)
}

func (s *moderationService) UnblockUser(ctx context.Context, blockerID, blockedID string) error {
	blocker, blocked, err := parseRelationship(blockerID, blockedID, "blocker id", "blocked id")
	if err != nil {
		return err
	}
	return s.repo.Unblock(ctx, blocker, blocked)
}

func (s *moderationService) MuteUser(ctx context.Context, muterID, mutedID string) error {
	muter, muted, err := parseRelationship(muterID, mutedID, "muter id", "muted id")
	if err != nil {
		return err
	}
	return s.repo.Mute(ctx, muter, muted)
}

func (s *moderationService) UnmuteUser(ctx context.Context, muterID, mutedID string) error {
	muter, muted, err := parseRelationship(muterID, mutedID, "muter id", "muted id")
	if err != nil {
		return err
	}
	return s.repo.Unmute(ctx, muter, muted)
}

func (s *moderationService) GetStatus(ctx context.Context, viewerID, targetID string) (models.Visibility, error) {
	viewer, target, err := parseRelationship(viewerID, targetID, "viewer id", "target id")
	if errors.Is(err, models.ErrCannotModerateSelf) {
		// A user cannot block themselves, so the honest answer is "visible to
		// yourself" rather than an error that leaks caller mistakes.
		return models.Visibility{}, nil
	}
	if err != nil {
		return models.Visibility{}, err
	}
	return s.repo.GetVisibility(ctx, viewer, target)
}

func (s *moderationService) ListBlocked(ctx context.Context, userID string) ([]string, error) {
	return s.listRelationships(ctx, userID, s.repo.ListBlocked)
}

func (s *moderationService) ListMuted(ctx context.Context, userID string) ([]string, error) {
	return s.listRelationships(ctx, userID, s.repo.ListMuted)
}

func (s *moderationService) listRelationships(
	ctx context.Context,
	userID string,
	list func(context.Context, uuid.UUID) ([]uuid.UUID, error),
) ([]string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid user id", models.ErrInvalidInput)
	}

	ids, err := list(ctx, parsed)
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(ids))
	for _, id := range ids {
		result = append(result, id.String())
	}
	return result, nil
}

func (s *moderationService) ReportContent(
	ctx context.Context,
	reporterID, targetType, targetID, reason, details string,
) (*models.Report, bool, error) {
	reporter, err := uuid.Parse(strings.TrimSpace(reporterID))
	if err != nil {
		return nil, false, fmt.Errorf("%w: invalid reporter id", models.ErrInvalidInput)
	}
	target, err := uuid.Parse(strings.TrimSpace(targetID))
	if err != nil {
		return nil, false, fmt.Errorf("%w: invalid target id", models.ErrInvalidInput)
	}

	targetType = strings.ToUpper(strings.TrimSpace(targetType))
	if !models.IsTargetType(targetType) {
		return nil, false, fmt.Errorf("%w: unsupported report target type", models.ErrInvalidInput)
	}
	reason = strings.ToUpper(strings.TrimSpace(reason))
	if !models.IsReason(reason) {
		return nil, false, fmt.Errorf("%w: unsupported report reason", models.ErrInvalidInput)
	}

	details = strings.TrimSpace(details)
	if utf8.RuneCountInString(details) > maxReportDetails {
		return nil, false, fmt.Errorf("%w: report details must be at most %d characters", models.ErrInvalidInput, maxReportDetails)
	}

	if targetType == models.TargetUser && target == reporter {
		return nil, false, models.ErrCannotModerateSelf
	}

	report, duplicate, err := s.repo.CreateReport(ctx, &models.Report{
		ReporterID: reporter,
		TargetType: targetType,
		TargetID:   target,
		Reason:     reason,
		Details:    details,
	})
	if err != nil {
		return nil, false, fmt.Errorf("failed to report content: %w", err)
	}
	return report, duplicate, nil
}

func (s *moderationService) ListReports(ctx context.Context, status string, limit int32, cursor string) (*models.ReportPage, error) {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "" && !models.IsOpenStatus(status) && status != models.StatusActioned && status != models.StatusDismissed {
		return nil, fmt.Errorf("%w: unsupported report status", models.ErrInvalidInput)
	}

	pageSize := int(limit)
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	position, err := models.DecodeReportCursor(cursor)
	if err != nil {
		return nil, err
	}

	reports, err := s.repo.ListReports(ctx, status, position, pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to list reports: %w", err)
	}

	page := &models.ReportPage{Reports: reports}
	if len(reports) > pageSize {
		page.Reports = reports[:pageSize]
		page.HasMore = true
		last := page.Reports[len(page.Reports)-1]
		page.NextCursor = models.EncodeReportCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

func (s *moderationService) ResolveReport(ctx context.Context, reportID, reviewerID, status, note string) (*models.Report, error) {
	reportUUID, err := uuid.Parse(strings.TrimSpace(reportID))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid report id", models.ErrInvalidInput)
	}
	reviewerUUID, err := uuid.Parse(strings.TrimSpace(reviewerID))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid reviewer id", models.ErrInvalidInput)
	}

	status = strings.ToUpper(strings.TrimSpace(status))
	if !models.IsResolvableStatus(status) {
		return nil, fmt.Errorf("%w: report can only move to REVIEWING, ACTIONED, or DISMISSED", models.ErrInvalidInput)
	}

	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > maxReportDetails {
		return nil, fmt.Errorf("%w: review note must be at most %d characters", models.ErrInvalidInput, maxReportDetails)
	}

	report, err := s.repo.ResolveReport(ctx, reportUUID, reviewerUUID, status, note)
	if err != nil {
		switch {
		case errors.Is(err, models.ErrReportNotFound), errors.Is(err, models.ErrReportClosed):
			return nil, err
		default:
			return nil, fmt.Errorf("failed to resolve report: %w", err)
		}
	}
	return report, nil
}
