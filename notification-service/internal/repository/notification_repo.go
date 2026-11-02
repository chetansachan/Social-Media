package repository

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NotificationRepository is the durable inbox. PostgreSQL is the system of
// record; Redis only caches counters and aggregation state.
type NotificationRepository interface {
	Create(ctx context.Context, input *models.CreateNotificationInput) (*models.Notification, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Notification, error)
	List(ctx context.Context, recipientID uuid.UUID, filter models.NotificationType, cursor models.Cursor, limit int) ([]models.Notification, error)
	MarkRead(ctx context.Context, recipientID uuid.UUID, ids []string) (int, error)
	MarkAllRead(ctx context.Context, recipientID uuid.UUID) (int, error)
	Delete(ctx context.Context, recipientID uuid.UUID, id uuid.UUID) (bool, error)
	CountUnread(ctx context.Context, recipientID uuid.UUID) (int, error)
	UpdateAggregate(ctx context.Context, id uuid.UUID, actorIDs []string, total int, title, preview string, reopen bool) (bool, error)
	EnsurePartitions(ctx context.Context, monthsAhead int) error
	PruneBefore(ctx context.Context, cutoff time.Time) (int64, error)
	DropPartitionsBefore(ctx context.Context, cutoff time.Time) (int, error)
}

type notificationRepository struct {
	db *pgxpool.Pool
}

func NewNotificationRepository(db *pgxpool.Pool) NotificationRepository {
	return &notificationRepository{db: db}
}

const notificationColumns = `
	id, recipient_id, type::text, entity_id, entity_type, actor_ids,
	total_actors_count, title, content_preview, action_url,
	is_read, read_at, created_at, updated_at, deleted_at
`

func scanNotification(row pgx.Row) (*models.Notification, error) {
	notification := &models.Notification{}
	var typeValue string

	if err := row.Scan(
		&notification.ID,
		&notification.RecipientID,
		&typeValue,
		&notification.EntityID,
		&notification.EntityType,
		&notification.ActorIDs,
		&notification.TotalActorsCount,
		&notification.Title,
		&notification.ContentPreview,
		&notification.ActionURL,
		&notification.IsRead,
		&notification.ReadAt,
		&notification.CreatedAt,
		&notification.UpdatedAt,
		&notification.DeletedAt,
	); err != nil {
		return nil, err
	}

	notification.Type = models.NotificationType(typeValue)
	if notification.ActorIDs == nil {
		notification.ActorIDs = []string{}
	}
	return notification, nil
}

func (r *notificationRepository) Create(ctx context.Context, input *models.CreateNotificationInput) (*models.Notification, error) {
	actorIDs := input.ActorIDs
	if actorIDs == nil {
		actorIDs = []string{}
	}
	total := input.TotalActorsCount
	if total < 1 {
		total = 1
	}

	query := `
		INSERT INTO notifications (
			recipient_id, type, entity_id, entity_type, actor_ids,
			total_actors_count, title, content_preview, action_url
		)
		VALUES ($1, $2::notification_type, $3, $4, $5, $6, $7, $8, $9)
		RETURNING ` + notificationColumns

	notification, err := scanNotification(r.db.QueryRow(ctx, query,
		input.RecipientID,
		string(input.Type),
		input.EntityID,
		input.EntityType,
		actorIDs,
		total,
		input.Title,
		input.ContentPreview,
		input.ActionURL,
	))
	if err != nil {
		return nil, fmt.Errorf("failed to insert notification: %w", err)
	}
	return notification, nil
}

func (r *notificationRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Notification, error) {
	query := `SELECT ` + notificationColumns + ` FROM notifications WHERE id = $1 AND deleted_at IS NULL`

	notification, err := scanNotification(r.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrNotificationNotFound
		}
		return nil, fmt.Errorf("failed to load notification: %w", err)
	}
	return notification, nil
}

func (r *notificationRepository) List(ctx context.Context, recipientID uuid.UUID, filter models.NotificationType, cursor models.Cursor, limit int) ([]models.Notification, error) {
	if limit <= 0 {
		limit = 20
	}

	args := []any{recipientID}
	where := "recipient_id = $1 AND deleted_at IS NULL"

	if filter != "" {
		args = append(args, string(filter))
		where += fmt.Sprintf(" AND type = $%d::notification_type", len(args))
	}

	if !cursor.IsZero() {
		args = append(args, cursor.CreatedAt, cursor.ID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}

	args = append(args, limit)
	query := fmt.Sprintf(`
		SELECT %s
		FROM notifications
		WHERE %s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d
	`, notificationColumns, where, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query notifications: %w", err)
	}
	defer rows.Close()

	notifications := make([]models.Notification, 0, limit)
	for rows.Next() {
		notification, err := scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification row: %w", err)
		}
		notifications = append(notifications, *notification)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notification row iteration failed: %w", err)
	}

	return notifications, nil
}

func (r *notificationRepository) MarkRead(ctx context.Context, recipientID uuid.UUID, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	parsed := make([]uuid.UUID, 0, len(ids))
	for _, raw := range ids {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return 0, fmt.Errorf("%w: invalid notification id %q", models.ErrInvalidInput, raw)
		}
		parsed = append(parsed, id)
	}

	query := `
		UPDATE notifications
		SET is_read = TRUE, read_at = NOW(), updated_at = NOW()
		WHERE recipient_id = $1
		  AND id = ANY($2::uuid[])
		  AND is_read = FALSE
		  AND deleted_at IS NULL
	`
	tag, err := r.db.Exec(ctx, query, recipientID, parsed)
	if err != nil {
		return 0, fmt.Errorf("failed to mark notifications as read: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *notificationRepository) MarkAllRead(ctx context.Context, recipientID uuid.UUID) (int, error) {
	query := `
		UPDATE notifications
		SET is_read = TRUE, read_at = NOW(), updated_at = NOW()
		WHERE recipient_id = $1 AND is_read = FALSE AND deleted_at IS NULL
	`
	tag, err := r.db.Exec(ctx, query, recipientID)
	if err != nil {
		return 0, fmt.Errorf("failed to mark all notifications as read: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// Delete soft-deletes an inbox row. It returns whether the row was unread so
// the caller can decrement the Redis badge without a second query.
func (r *notificationRepository) Delete(ctx context.Context, recipientID uuid.UUID, id uuid.UUID) (bool, error) {
	query := `
		UPDATE notifications
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND recipient_id = $2 AND deleted_at IS NULL
		RETURNING is_read
	`
	var isRead bool
	if err := r.db.QueryRow(ctx, query, id, recipientID).Scan(&isRead); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, models.ErrNotificationNotFound
		}
		return false, fmt.Errorf("failed to delete notification: %w", err)
	}
	return !isRead, nil
}

func (r *notificationRepository) CountUnread(ctx context.Context, recipientID uuid.UUID) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM notifications
		WHERE recipient_id = $1 AND is_read = FALSE AND deleted_at IS NULL
	`
	var count int
	if err := r.db.QueryRow(ctx, query, recipientID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count unread notifications: %w", err)
	}
	return count, nil
}

// UpdateAggregate rewrites the actor preview/count of an existing notification.
// When reopen is true the row is marked unread again (a like arriving after the
// user read the previous one), which callers use to increment the badge.
func (r *notificationRepository) UpdateAggregate(ctx context.Context, id uuid.UUID, actorIDs []string, total int, title, preview string, reopen bool) (bool, error) {
	if actorIDs == nil {
		actorIDs = []string{}
	}
	if total < 0 {
		total = 0
	}

	setClause := `
		actor_ids = $2,
		total_actors_count = $3,
		title = $4,
		content_preview = $5,
		updated_at = NOW()`
	if reopen {
		setClause += `, is_read = FALSE, read_at = NULL`
	}

	query := fmt.Sprintf(`
		WITH previous AS (
			SELECT is_read
			FROM notifications
			WHERE id = $1 AND deleted_at IS NULL
			FOR UPDATE
		)
		UPDATE notifications AS n
		SET %s
		FROM previous
		WHERE n.id = $1
		RETURNING previous.is_read
	`, setClause)

	var wasRead bool
	if err := r.db.QueryRow(ctx, query, id, actorIDs, total, title, preview).Scan(&wasRead); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, models.ErrNotificationNotFound
		}
		return false, fmt.Errorf("failed to update notification aggregate: %w", err)
	}
	return wasRead, nil
}

// EnsurePartitions creates the current month and the requested number of
// future monthly partitions. It is idempotent and safe to run on every boot.
func (r *notificationRepository) EnsurePartitions(ctx context.Context, monthsAhead int) error {
	if monthsAhead < 1 {
		monthsAhead = 1
	}

	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i <= monthsAhead; i++ {
		start := monthStart.AddDate(0, i, 0)
		end := start.AddDate(0, 1, 0)
		name := "notifications_" + start.Format("2006_01")

		query := fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s PARTITION OF notifications FOR VALUES FROM ('%s') TO ('%s')`,
			pgx.Identifier{name}.Sanitize(),
			start.Format("2006-01-02 15:04:05Z07:00"),
			end.Format("2006-01-02 15:04:05Z07:00"),
		)
		if _, err := r.db.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to create partition %s: %w", name, err)
		}
	}
	return nil
}

// PruneBefore deletes old rows in bounded batches. This handles the partially
// expired month that cannot be dropped as a whole partition.
func (r *notificationRepository) PruneBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	const batchSize = 10000
	var total int64

	for {
		query := `
			DELETE FROM notifications
			WHERE (id, created_at) IN (
				SELECT id, created_at
				FROM notifications
				WHERE created_at < $1
				LIMIT $2
			)
		`
		tag, err := r.db.Exec(ctx, query, cutoff, batchSize)
		if err != nil {
			return total, fmt.Errorf("failed to prune notifications: %w", err)
		}
		deleted := tag.RowsAffected()
		total += deleted
		if deleted == 0 {
			return total, nil
		}
	}
}

var partitionNamePattern = regexp.MustCompile(`^notifications_(\d{4})_(\d{2})$`)

// DropPartitionsBefore drops monthly partitions whose entire range is older
// than cutoff. Dropping a partition is dramatically cheaper than deleting rows
// and is the primary retention mechanism.
func (r *notificationRepository) DropPartitionsBefore(ctx context.Context, cutoff time.Time) (int, error) {
	query := `
		SELECT child.relname
		FROM pg_inherits
		JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
		JOIN pg_class child ON pg_inherits.inhrelid = child.oid
		JOIN pg_namespace ns ON child.relnamespace = ns.oid
		WHERE parent.relname = 'notifications'
		  AND ns.nspname = current_schema()
		  AND child.relname LIKE 'notifications\_%'
	`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to list notification partitions: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return 0, fmt.Errorf("failed to scan partition name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("partition iteration failed: %w", err)
	}

	dropped := 0
	for _, name := range names {
		matches := partitionNamePattern.FindStringSubmatch(name)
		if matches == nil {
			continue
		}

		var year, month int
		if _, err := fmt.Sscanf(matches[1]+"-"+matches[2], "%d-%d", &year, &month); err != nil {
			continue
		}
		start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0)
		if end.After(cutoff) {
			continue
		}

		if _, err := r.db.Exec(ctx, "DROP TABLE IF EXISTS "+pgx.Identifier{name}.Sanitize()); err != nil {
			return dropped, fmt.Errorf("failed to drop partition %s: %w", name, err)
		}
		dropped++
	}

	return dropped, nil
}
