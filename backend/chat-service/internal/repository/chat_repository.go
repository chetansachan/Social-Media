package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chetansachan/socialflow/chat-service/internal/database"
	"github.com/chetansachan/socialflow/chat-service/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConversationSummary is a models type: the thread joined with its members,
// last message, and the viewer's unread count. Sharing it with the service lets
// the inbox and detail reads return one row shape.

type ChatRepository interface {
	CreateDirectConversation(ctx context.Context, creatorID, otherID uuid.UUID) (*models.Conversation, error)
	CreateGroupConversation(ctx context.Context, creatorID uuid.UUID, title string, participantIDs []uuid.UUID) (*models.Conversation, error)
	GetConversation(ctx context.Context, id uuid.UUID) (*models.Conversation, error)
	IsParticipant(ctx context.Context, conversationID, userID uuid.UUID) (bool, error)
	GetParticipants(ctx context.Context, conversationID uuid.UUID) ([]models.Participant, error)
	LastMessageFor(ctx context.Context, conversationID uuid.UUID) (*models.Message, error)
	UnreadCountFor(ctx context.Context, conversationID, viewerID uuid.UUID) (int, error)
	// ListConversations may return one more row than the caller asked for so the
	// caller can decide has_more without a second COUNT query.
	ListConversations(ctx context.Context, viewerID uuid.UUID, cursor models.ConversationCursor, limit int, excluded []uuid.UUID) ([]models.ConversationSummary, error)
	GetMessages(ctx context.Context, conversationID, viewerID uuid.UUID, cursor models.MessageCursor, limit int) ([]models.Message, error)
	SendMessage(ctx context.Context, conversationID, senderID uuid.UUID, content string) (*models.Message, error)
	DeleteMessage(ctx context.Context, messageID, senderID uuid.UUID) (*models.Message, error)
	MarkRead(ctx context.Context, conversationID, userID uuid.UUID) (*time.Time, error)
	TotalUnread(ctx context.Context, viewerID uuid.UUID, excluded []uuid.UUID) (int, error)
}

type chatRepository struct {
	writer *pgxpool.Pool
	reader *pgxpool.Pool
}

/* ---- Unread counts -------------------------------------------------------
   Three shapes, one rule: a message the viewer wrote is never unread, because
   sending advances no read receipt. Every query therefore filters
   `m.sender_id <> $viewer`, reusing the viewer binding already present.

   They are named so the invariant is stated once and is inspectable in a test,
   rather than being buried in three inline strings that can drift apart. */

// $1 conversation, $2 viewer.
const queryUnreadCountForConversation = `
	SELECT COUNT(*)::int
	FROM messages m
	JOIN conversation_participants p
	  ON p.conversation_id = m.conversation_id AND p.user_id = $2
	WHERE m.conversation_id = $1
	  AND m.deleted_at IS NULL
	  AND m.sender_id <> $2
	  AND (p.last_read_at IS NULL OR m.created_at > p.last_read_at)
`

// $1 viewer, $2 conversation ids.
const queryUnreadCountsForMany = `
	SELECT m.conversation_id, COUNT(*)::int
	FROM messages m
	JOIN conversation_participants p
	  ON p.conversation_id = m.conversation_id AND p.user_id = $1
	WHERE m.conversation_id = ANY($2::uuid[])
	  AND m.deleted_at IS NULL
	  AND m.sender_id <> $1
	  AND (p.last_read_at IS NULL OR m.created_at > p.last_read_at)
	GROUP BY m.conversation_id
`

// $1 viewer, $2 excluded (blocked) user ids.
const queryTotalUnread = `
	SELECT COALESCE(SUM(counts.unread), 0)::int
	FROM (
	    SELECT m.conversation_id, COUNT(*) AS unread
	    FROM messages m
	    JOIN conversation_participants p
	      ON p.conversation_id = m.conversation_id AND p.user_id = $1
	    WHERE m.deleted_at IS NULL
	      AND m.sender_id <> $1
	      AND (p.last_read_at IS NULL OR m.created_at > p.last_read_at)
	      AND NOT EXISTS (
	          SELECT 1 FROM conversation_participants hidden
	          WHERE hidden.conversation_id = m.conversation_id
	            AND hidden.user_id = ANY($2::uuid[])
	      )
	    GROUP BY m.conversation_id
	) counts
`

func NewChatRepository(db *database.DB) ChatRepository {
	return &chatRepository{
		writer: db.Writer(),
		reader: db.Reader(),
	}
}

const conversationColumns = `id, type, title, created_by,
	COALESCE(last_message_at, created_at), created_at, updated_at`

func scanConversation(row interface{ Scan(dest ...any) error }) (*models.Conversation, error) {
	conversation := &models.Conversation{}
	if err := row.Scan(
		&conversation.ID,
		&conversation.Type,
		&conversation.Title,
		&conversation.CreatedBy,
		&conversation.LastMessageAt,
		&conversation.CreatedAt,
		&conversation.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return conversation, nil
}

// directKey orders two ids so the pair maps to one stable string regardless of
// who started the thread.
func directKey(first, second uuid.UUID) string {
	if first.String() < second.String() {
		return first.String() + "|" + second.String()
	}
	return second.String() + "|" + first.String()
}

// CreateDirectConversation returns the existing thread for a pair when one
// exists. The unique partial index makes the ON CONFLICT safe, so two members
// clicking "message" at the same moment still produce one thread.
func (r *chatRepository) CreateDirectConversation(ctx context.Context, creatorID, otherID uuid.UUID) (*models.Conversation, error) {
	return r.createConversation(ctx, models.ConversationDirect, "", creatorID, nil, &otherID)
}

// CreateGroupConversation opens a named thread with the creator plus members.
func (r *chatRepository) CreateGroupConversation(
	ctx context.Context,
	creatorID uuid.UUID,
	title string,
	participantIDs []uuid.UUID,
) (*models.Conversation, error) {
	return r.createConversation(ctx, models.ConversationGroup, title, creatorID, participantIDs, nil)
}

func (r *chatRepository) createConversation(
	ctx context.Context,
	kind string,
	title string,
	creatorID uuid.UUID,
	participantIDs []uuid.UUID,
	directOther *uuid.UUID,
) (*models.Conversation, error) {
	tx, err := r.writer.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin create conversation transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	key := ""
	if directOther != nil {
		key = directKey(creatorID, *directOther)
	}

	conversation, err := scanConversation(tx.QueryRow(ctx, `
		INSERT INTO conversations (type, title, created_by, direct_key)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		ON CONFLICT (direct_key) WHERE direct_key IS NOT NULL DO NOTHING
		RETURNING `+conversationColumns,
		kind, title, creatorID, key,
	))
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("failed to create conversation: %w", err)
		}

		// The pair already has a thread: return the stored one instead of a
		// second row for the same two people.
		lookup, scanErr := scanConversation(tx.QueryRow(ctx, `
			SELECT `+conversationColumns+` FROM conversations WHERE direct_key = NULLIF($1, '')
		`, key))
		if scanErr != nil {
			return nil, fmt.Errorf("failed to look up existing conversation: %w", scanErr)
		}
		return lookup, nil
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO conversation_participants (conversation_id, user_id)
		VALUES ($1, $2)
	`, conversation.ID, creatorID); err != nil {
		return nil, fmt.Errorf("failed to add creator to conversation: %w", err)
	}

	if directOther != nil {
		if _, err := tx.Exec(ctx, `
			INSERT INTO conversation_participants (conversation_id, user_id)
			VALUES ($1, $2)
		`, conversation.ID, *directOther); err != nil {
			return nil, fmt.Errorf("failed to add direct participant: %w", err)
		}
	}

	for _, id := range participantIDs {
		if id == creatorID {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO conversation_participants (conversation_id, user_id)
			VALUES ($1, $2)
			ON CONFLICT (conversation_id, user_id) DO NOTHING
		`, conversation.ID, id); err != nil {
			return nil, fmt.Errorf("failed to add group participant: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit create conversation transaction: %w", err)
	}

	return conversation, nil
}

func (r *chatRepository) GetConversation(ctx context.Context, id uuid.UUID) (*models.Conversation, error) {
	conversation, err := scanConversation(r.reader.QueryRow(ctx, `
		SELECT `+conversationColumns+` FROM conversations WHERE id = $1
	`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrConversationNotFound
		}
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}
	return conversation, nil
}

// IsParticipant is the check every other method runs first: a caller is never
// handed a conversation, a message, or a membership row they do not belong to.
func (r *chatRepository) IsParticipant(ctx context.Context, conversationID, userID uuid.UUID) (bool, error) {
	var exists bool
	if err := r.reader.QueryRow(ctx, `
		SELECT EXISTS (
		    SELECT 1 FROM conversation_participants
		    WHERE conversation_id = $1 AND user_id = $2
		)
	`, conversationID, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check membership: %w", err)
	}
	return exists, nil
}

// GetParticipants lists a thread's members, which is the fan-out list for
// realtime frames.
func (r *chatRepository) GetParticipants(ctx context.Context, conversationID uuid.UUID) ([]models.Participant, error) {
	rows, err := r.reader.Query(ctx, `
		SELECT user_id, last_read_at
		FROM conversation_participants
		WHERE conversation_id = $1
		ORDER BY joined_at ASC, user_id ASC
	`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to list participants: %w", err)
	}
	defer rows.Close()

	participants := make([]models.Participant, 0, 4)
	for rows.Next() {
		var participant models.Participant
		if err := rows.Scan(&participant.UserID, &participant.LastReadAt); err != nil {
			return nil, fmt.Errorf("failed to scan participant: %w", err)
		}
		participants = append(participants, participant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating participants: %w", err)
	}
	return participants, nil
}

// LastMessageFor returns the newest non-tombstone message, or nil for an empty
// or fully deleted thread.
func (r *chatRepository) LastMessageFor(ctx context.Context, conversationID uuid.UUID) (*models.Message, error) {
	messages, err := r.GetMessages(ctx, conversationID, uuid.Nil, models.MessageCursor{}, 1)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, nil
	}
	if messages[0].Content == "" {
		return nil, nil
	}
	return &messages[0], nil
}

// UnreadCountFor counts live messages newer than the viewer's watermark. A NULL
// watermark means nothing has been read, so the whole thread is unread.
//
// A message the viewer wrote is never unread: sending advances no read receipt,
// so without the sender filter the author's own messages would inflate their
// own badge. `$2` is the viewer, and the joined participant row proves
// membership of that same person.
func (r *chatRepository) UnreadCountFor(ctx context.Context, conversationID, viewerID uuid.UUID) (int, error) {
	var count int
	if err := r.reader.QueryRow(ctx, queryUnreadCountForConversation, conversationID, viewerID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count unread: %w", err)
	}
	return count, nil
}

// ListConversations returns the viewer's inbox newest-activity first. Excluded
// ids hide any thread that includes a blocked user, which is how the gateway
// enforces a block without ever naming who set it. Callers pass limit+1 as the
// limit to learn has_more without a separate COUNT.
func (r *chatRepository) ListConversations(
	ctx context.Context,
	viewerID uuid.UUID,
	cursor models.ConversationCursor,
	limit int,
	excluded []uuid.UUID,
) ([]models.ConversationSummary, error) {
	if limit <= 0 {
		limit = 21
	}

	var lastActivity *time.Time
	if !cursor.IsZero() {
		at := cursor.LastMessageAt
		lastActivity = &at
	}

	rows, err := r.reader.Query(ctx, `
		SELECT `+conversationColumns+`
		FROM conversations c
		JOIN conversation_participants p ON p.conversation_id = c.id
		WHERE p.user_id = $1
		  AND ($2::timestamptz IS NULL OR (COALESCE(c.last_message_at, c.created_at), c.id) < ($2, $3))
		  AND NOT EXISTS (
		      SELECT 1 FROM conversation_participants hidden
		      WHERE hidden.conversation_id = c.id
		        AND hidden.user_id = ANY($4::uuid[])
		  )
		ORDER BY COALESCE(c.last_message_at, c.created_at) DESC, c.id DESC
		LIMIT $5
	`, viewerID, lastActivity, cursor.ID, excluded, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list conversations: %w", err)
	}
	defer rows.Close()

	conversations := make([]models.Conversation, 0, limit)
	for rows.Next() {
		conversation, err := scanConversation(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan conversation: %w", err)
		}
		conversations = append(conversations, *conversation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating conversations: %w", err)
	}

	// Three batched lookups hydrate the whole page instead of one query per row.
	ids := make([]uuid.UUID, 0, len(conversations))
	for _, conversation := range conversations {
		ids = append(ids, conversation.ID)
	}

	participants, err := r.participantsFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	lastMessages, err := r.lastMessagesFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	unread, err := r.unreadCountsFor(ctx, viewerID, ids)
	if err != nil {
		return nil, err
	}

	results := make([]models.ConversationSummary, 0, len(conversations))
	for _, conversation := range conversations {
		meta := models.ConversationSummary{
			Conversation: conversation,
			Participants: participants[conversation.ID],
			UnreadCount:  unread[conversation.ID],
		}
		if lastMessage, ok := lastMessages[conversation.ID]; ok {
			meta.LastMessage = lastMessage
		}
		results = append(results, meta)
	}
	return results, nil
}

func (r *chatRepository) participantsFor(ctx context.Context, conversationIDs []uuid.UUID) (map[uuid.UUID][]models.Participant, error) {
	grouped := make(map[uuid.UUID][]models.Participant, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return grouped, nil
	}

	rows, err := r.reader.Query(ctx, `
		SELECT conversation_id, user_id, last_read_at
		FROM conversation_participants
		WHERE conversation_id = ANY($1::uuid[])
		ORDER BY joined_at ASC, user_id ASC
	`, conversationIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to batch list participants: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			conversationID uuid.UUID
			participant    models.Participant
		)
		if err := rows.Scan(&conversationID, &participant.UserID, &participant.LastReadAt); err != nil {
			return nil, fmt.Errorf("failed to scan batched participant: %w", err)
		}
		grouped[conversationID] = append(grouped[conversationID], participant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating batched participants: %w", err)
	}
	return grouped, nil
}

func (r *chatRepository) lastMessagesFor(ctx context.Context, conversationIDs []uuid.UUID) (map[uuid.UUID]*models.Message, error) {
	grouped := make(map[uuid.UUID]*models.Message, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return grouped, nil
	}

	rows, err := r.reader.Query(ctx, `
		SELECT DISTINCT ON (conversation_id)
		       conversation_id, id, sender_id, content, created_at, deleted_at IS NOT NULL
		FROM messages
		WHERE conversation_id = ANY($1::uuid[])
		ORDER BY conversation_id, created_at DESC, id DESC
	`, conversationIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to batch list last messages: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			conversationID uuid.UUID
			message        models.Message
			isDeleted      bool
		)
		if err := rows.Scan(
			&conversationID,
			&message.ID,
			&message.SenderID,
			&message.Content,
			&message.CreatedAt,
			&isDeleted,
		); err != nil {
			return nil, fmt.Errorf("failed to scan batched last message: %w", err)
		}
		// A tombstone is not worth showing as the inbox preview.
		if isDeleted {
			continue
		}
		grouped[conversationID] = &message
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating batched last messages: %w", err)
	}
	return grouped, nil
}

func (r *chatRepository) unreadCountsFor(
	ctx context.Context,
	viewerID uuid.UUID,
	conversationIDs []uuid.UUID,
) (map[uuid.UUID]int, error) {
	counts := make(map[uuid.UUID]int, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return counts, nil
	}

	rows, err := r.reader.Query(ctx, queryUnreadCountsForMany, viewerID, conversationIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to batch count unread: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var conversationID uuid.UUID
		var count int
		if err := rows.Scan(&conversationID, &count); err != nil {
			return nil, fmt.Errorf("failed to scan unread count: %w", err)
		}
		counts[conversationID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating unread counts: %w", err)
	}
	return counts, nil
}

// TotalUnread sums the badge across the viewer's threads, minus hidden ones.
func (r *chatRepository) TotalUnread(ctx context.Context, viewerID uuid.UUID, excluded []uuid.UUID) (int, error) {
	var total int
	if err := r.reader.QueryRow(ctx, queryTotalUnread, viewerID, excluded).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count total unread: %w", err)
	}
	return total, nil
}

// SendMessage inserts a message and bumps the thread's last activity in one
// transaction, so an inbox can never show a newer message under an older
// sort key.
func (r *chatRepository) SendMessage(ctx context.Context, conversationID, senderID uuid.UUID, content string) (*models.Message, error) {
	tx, err := r.writer.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin send message transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	message := &models.Message{}
	if err := tx.QueryRow(ctx, `
		INSERT INTO messages (conversation_id, sender_id, content)
		VALUES ($1, $2, $3)
		RETURNING id, conversation_id, sender_id, content, created_at, deleted_at
	`, conversationID, senderID, content).Scan(
		&message.ID,
		&message.ConversationID,
		&message.SenderID,
		&message.Content,
		&message.CreatedAt,
		&message.DeletedAt,
	); err != nil {
		return nil, fmt.Errorf("failed to insert message: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE conversations
		SET last_message_at = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`, message.CreatedAt, conversationID); err != nil {
		return nil, fmt.Errorf("failed to bump conversation activity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit send message transaction: %w", err)
	}

	return message, nil
}

func (r *chatRepository) GetMessages(
	ctx context.Context,
	conversationID, viewerID uuid.UUID,
	cursor models.MessageCursor,
	limit int,
) ([]models.Message, error) {
	if limit <= 0 {
		limit = 21
	}

	var createdAt *time.Time
	if !cursor.IsZero() {
		at := cursor.CreatedAt
		createdAt = &at
	}

	rows, err := r.reader.Query(ctx, `
		SELECT id, conversation_id, sender_id, content, created_at, deleted_at IS NOT NULL
		FROM messages
		WHERE conversation_id = $1
		  AND ($2::timestamptz IS NULL OR (created_at, id) < ($2, $3))
		ORDER BY created_at DESC, id DESC
		LIMIT $4
	`, conversationID, createdAt, cursor.ID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}
	defer rows.Close()

	messages := make([]models.Message, 0, limit)
	for rows.Next() {
		var (
			message   models.Message
			isDeleted bool
		)
		if err := rows.Scan(
			&message.ID,
			&message.ConversationID,
			&message.SenderID,
			&message.Content,
			&message.CreatedAt,
			&isDeleted,
		); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		if isDeleted {
			message.Content = ""
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}
	return messages, nil
}

// DeleteMessage soft-deletes a sender's own message. A tombstone keeps the
// timeline stable for the other side, so only the text is cleared.
func (r *chatRepository) DeleteMessage(ctx context.Context, messageID, senderID uuid.UUID) (*models.Message, error) {
	message, err := scanMessage(r.writer.QueryRow(ctx, `
		UPDATE messages
		SET content = '', deleted_at = CURRENT_TIMESTAMP, deleted_by = $2
		WHERE id = $1 AND sender_id = $2 AND deleted_at IS NULL
		RETURNING id, conversation_id, sender_id, content, created_at, deleted_at IS NOT NULL
	`, messageID, senderID))
	if err == nil {
		return message, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to delete message: %w", err)
	}

	// No row updated: distinguish "already gone" from "not yours" so the
	// handler can choose its code.
	existing, lookupErr := scanMessage(r.reader.QueryRow(ctx, `
		SELECT id, conversation_id, sender_id, content, created_at, deleted_at IS NOT NULL
		FROM messages WHERE id = $1
	`, messageID))
	if errors.Is(lookupErr, pgx.ErrNoRows) {
		return nil, models.ErrMessageNotFound
	}
	if lookupErr != nil {
		return nil, fmt.Errorf("failed to look up message after delete: %w", lookupErr)
	}
	if existing.SenderID != senderID {
		return nil, models.ErrNotParticipant
	}
	if existing.Content == "" {
		return nil, models.ErrMessageNotFound
	}
	return existing, nil
}

func scanMessage(row interface{ Scan(dest ...any) error }) (*models.Message, error) {
	message := &models.Message{}
	var isDeleted bool
	if err := row.Scan(
		&message.ID,
		&message.ConversationID,
		&message.SenderID,
		&message.Content,
		&message.CreatedAt,
		&isDeleted,
	); err != nil {
		return nil, err
	}
	return message, nil
}

// MarkRead advances the member's watermark to now, never backwards.
func (r *chatRepository) MarkRead(ctx context.Context, conversationID, userID uuid.UUID) (*time.Time, error) {
	now := time.Now().UTC()
	if _, err := r.writer.Exec(ctx, `
		UPDATE conversation_participants
		SET last_read_at = GREATEST(COALESCE(last_read_at, 'epoch'::timestamptz), $1)
		WHERE conversation_id = $2 AND user_id = $3
	`, now, conversationID, userID); err != nil {
		return nil, fmt.Errorf("failed to mark conversation read: %w", err)
	}
	return &now, nil
}
