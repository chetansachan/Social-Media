package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chetansachan/socialflow/moderation-service/internal/database"
	"github.com/chetansachan/socialflow/moderation-service/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ModerationRepository owns the block/mute graph and the report queue. Reads
// use the replica; every mutation runs on the primary.
type ModerationRepository interface {
	Block(ctx context.Context, blockerID, blockedID uuid.UUID) error
	Unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error
	Mute(ctx context.Context, muterID, mutedID uuid.UUID) error
	Unmute(ctx context.Context, muterID, mutedID uuid.UUID) error
	ListBlocked(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	ListMuted(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	GetVisibility(ctx context.Context, viewerID, targetID uuid.UUID) (models.Visibility, error)
	CreateReport(ctx context.Context, report *models.Report) (stored *models.Report, duplicate bool, err error)
	ListReports(ctx context.Context, status string, cursor models.ReportCursor, limit int) ([]models.Report, error)
	ResolveReport(ctx context.Context, reportID, reviewerID uuid.UUID, status, note string) (*models.Report, error)
}

type moderationRepository struct {
	writer *pgxpool.Pool
	reader *pgxpool.Pool
}

func NewModerationRepository(db *database.DB) ModerationRepository {
	return &moderationRepository{
		writer: db.Writer(),
		reader: db.Reader(),
	}
}

func (r *moderationRepository) Block(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	if _, err := r.writer.Exec(ctx, `
		INSERT INTO user_blocks (blocker_id, blocked_id)
		VALUES ($1, $2)
		ON CONFLICT (blocker_id, blocked_id) DO NOTHING
	`, blockerID, blockedID); err != nil {
		return fmt.Errorf("failed to block user: %w", err)
	}
	return nil
}

func (r *moderationRepository) Unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	if _, err := r.writer.Exec(ctx, `
		DELETE FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2
	`, blockerID, blockedID); err != nil {
		return fmt.Errorf("failed to unblock user: %w", err)
	}
	return nil
}

func (r *moderationRepository) Mute(ctx context.Context, muterID, mutedID uuid.UUID) error {
	if _, err := r.writer.Exec(ctx, `
		INSERT INTO user_mutes (muter_id, muted_id)
		VALUES ($1, $2)
		ON CONFLICT (muter_id, muted_id) DO NOTHING
	`, muterID, mutedID); err != nil {
		return fmt.Errorf("failed to mute user: %w", err)
	}
	return nil
}

func (r *moderationRepository) Unmute(ctx context.Context, muterID, mutedID uuid.UUID) error {
	if _, err := r.writer.Exec(ctx, `
		DELETE FROM user_mutes WHERE muter_id = $1 AND muted_id = $2
	`, muterID, mutedID); err != nil {
		return fmt.Errorf("failed to unmute user: %w", err)
	}
	return nil
}

func (r *moderationRepository) ListBlocked(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	return r.listIDs(ctx, `
		SELECT blocked_id FROM user_blocks WHERE blocker_id = $1 ORDER BY created_at DESC
	`, userID)
}

func (r *moderationRepository) ListMuted(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	return r.listIDs(ctx, `
		SELECT muted_id FROM user_mutes WHERE muter_id = $1 ORDER BY created_at DESC
	`, userID)
}

func (r *moderationRepository) listIDs(ctx context.Context, query string, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.reader.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list moderation relationships: %w", err)
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0, 16)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan moderation relationship: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating moderation relationships: %w", err)
	}
	return ids, nil
}

// GetVisibility answers the three questions the gateway needs in one round
// trip instead of three.
func (r *moderationRepository) GetVisibility(ctx context.Context, viewerID, targetID uuid.UUID) (models.Visibility, error) {
	var visibility models.Visibility
	if err := r.reader.QueryRow(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2),
			EXISTS (SELECT 1 FROM user_blocks WHERE blocker_id = $2 AND blocked_id = $1),
			EXISTS (SELECT 1 FROM user_mutes WHERE muter_id = $1 AND muted_id = $2)
	`, viewerID, targetID).Scan(
		&visibility.IsBlocked,
		&visibility.HasBlockedViewer,
		&visibility.IsMuted,
	); err != nil {
		return models.Visibility{}, fmt.Errorf("failed to get moderation status: %w", err)
	}
	return visibility, nil
}

const reportColumns = `id, reporter_id, target_type, target_id, reason, details, status,
	created_at, reviewed_at, reviewer_id, review_note`

func scanReport(row interface{ Scan(dest ...any) error }) (*models.Report, error) {
	report := &models.Report{}
	if err := row.Scan(
		&report.ID,
		&report.ReporterID,
		&report.TargetType,
		&report.TargetID,
		&report.Reason,
		&report.Details,
		&report.Status,
		&report.CreatedAt,
		&report.ReviewedAt,
		&report.ReviewerID,
		&report.ReviewNote,
	); err != nil {
		return nil, err
	}
	return report, nil
}

// CreateReport records a flag unless the reporter already has an open report
// for the same target. The partial unique index makes that check atomic, so a
// double-submit returns the original report instead of a duplicate row.
func (r *moderationRepository) CreateReport(ctx context.Context, report *models.Report) (*models.Report, bool, error) {
	stored, err := scanReport(r.writer.QueryRow(ctx, `
		INSERT INTO content_reports (reporter_id, target_type, target_id, reason, details)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (reporter_id, target_type, target_id)
			WHERE status IN ('PENDING', 'REVIEWING')
			DO NOTHING
		RETURNING `+reportColumns,
		report.ReporterID, report.TargetType, report.TargetID, report.Reason, report.Details,
	))
	if err == nil {
		return stored, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("failed to create report: %w", err)
	}

	existing, err := scanReport(r.writer.QueryRow(ctx, `
		SELECT `+reportColumns+`
		FROM content_reports
		WHERE reporter_id = $1 AND target_type = $2 AND target_id = $3
		  AND status IN ('PENDING', 'REVIEWING')
		ORDER BY created_at DESC
		LIMIT 1
	`, report.ReporterID, report.TargetType, report.TargetID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, errors.New("report insert conflicted but no open report was found")
		}
		return nil, false, fmt.Errorf("failed to load existing report: %w", err)
	}
	return existing, true, nil
}

// ListReports pages the review queue newest-first. An empty status means the
// open queue (PENDING plus REVIEWING), which is what a moderator starts with.
func (r *moderationRepository) ListReports(
	ctx context.Context,
	status string,
	cursor models.ReportCursor,
	limit int,
) ([]models.Report, error) {
	if limit <= 0 {
		limit = 20
	}
	// Fetch one extra row to decide has_more without a COUNT.
	fetch := limit + 1

	rows, err := r.reader.Query(ctx, `
		SELECT `+reportColumns+`
		FROM content_reports
		WHERE (($1 = '' AND status IN ('PENDING', 'REVIEWING')) OR status = $1)
		  AND ($2::timestamptz IS NULL OR (created_at, id) < ($2, $3))
		ORDER BY created_at DESC, id DESC
		LIMIT $4
	`, status, nullableTime(cursor), cursor.ID, fetch)
	if err != nil {
		return nil, fmt.Errorf("failed to list reports: %w", err)
	}
	defer rows.Close()

	reports := make([]models.Report, 0, fetch)
	for rows.Next() {
		report, err := scanReport(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan report: %w", err)
		}
		reports = append(reports, *report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating reports: %w", err)
	}
	return reports, nil
}

// ResolveReport moves an open report to a terminal (or in-review) state and
// records who decided it.
func (r *moderationRepository) ResolveReport(
	ctx context.Context,
	reportID, reviewerID uuid.UUID,
	status, note string,
) (*models.Report, error) {
	report, err := scanReport(r.writer.QueryRow(ctx, `
		UPDATE content_reports
		SET status = $2, reviewer_id = $3, review_note = $4, reviewed_at = NOW()
		WHERE id = $1 AND status IN ('PENDING', 'REVIEWING')
		RETURNING `+reportColumns,
		reportID, status, reviewerID, note,
	))
	if err == nil {
		return report, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to resolve report: %w", err)
	}

	// No row updated: tell an already-resolved report apart from a missing one.
	var exists bool
	if lookupErr := r.writer.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM content_reports WHERE id = $1)
	`, reportID).Scan(&exists); lookupErr != nil {
		return nil, fmt.Errorf("failed to check report after update: %w", lookupErr)
	}
	if exists {
		return nil, models.ErrReportClosed
	}
	return nil, models.ErrReportNotFound
}

// nullableTime turns an unset cursor into SQL NULL so the first page needs no
// special query.
func nullableTime(cursor models.ReportCursor) *time.Time {
	if cursor.IsZero() {
		return nil
	}
	at := cursor.CreatedAt
	return &at
}
