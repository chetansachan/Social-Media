package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/chetansachan/socialflow/chat-service/internal/models"
	"github.com/chetansachan/socialflow/chat-service/internal/presence"
	"github.com/chetansachan/socialflow/chat-service/internal/realtime"
	"github.com/chetansachan/socialflow/chat-service/internal/repository"
	"github.com/google/uuid"
)

type ChatService interface {
	CreateConversation(ctx context.Context, creatorID string, participantIDs []string, title string) (*models.ConversationSummary, error)
	GetConversation(ctx context.Context, conversationID, viewerID string) (*models.ConversationSummary, error)
	ListConversations(ctx context.Context, viewerID string, limit int32, cursor string, excludedUserIDs []string) (*models.InboxPage, error)
	SendMessage(ctx context.Context, conversationID, senderID, content string) (*models.Message, error)
	GetMessages(ctx context.Context, conversationID, viewerID string, limit int32, cursor string) (*models.MessagePage, error)
	DeleteMessage(ctx context.Context, messageID, senderID string) (*models.Message, error)
	MarkRead(ctx context.Context, conversationID, userID string) error
	TotalUnread(ctx context.Context, viewerID string, excluded []uuid.UUID) (int, error)
	SetTyping(ctx context.Context, conversationID, userID string, isTyping bool) ([]string, error)
	Heartbeat(ctx context.Context, userID string) error
	GetPresence(ctx context.Context, userIDs []string) (map[string]int64, error)
}

const (
	maxMessageLength = 4000
	maxGroupMembers  = 200
	maxGroupTitle    = 100
	defaultPageSize  = 20
	maxPageSize      = 50
)

type chatService struct {
	repo repository.ChatRepository

	// Both are optional: a deployment without Redis still serves chat, it just
	// has no presence or typing signals and no realtime relay.
	presence  *presence.Store
	publisher *realtime.Publisher
}

// NewChatService wires the durable store to the ephemeral ones. A nil presence
// or publisher is allowed so tests can exercise the message path alone.
func NewChatService(
	repo repository.ChatRepository,
	presenceStore *presence.Store,
	publisher *realtime.Publisher,
) ChatService {
	return &chatService{repo: repo, presence: presenceStore, publisher: publisher}
}

// parseUUIDs parses and de-duplicates ids in order, skipping anything malformed
// so one bad id cannot fail a whole request.
func parseUUIDs(values []string) []uuid.UUID {
	parsed := make([]uuid.UUID, 0, len(values))
	seen := make(map[uuid.UUID]struct{}, len(values))
	for _, raw := range values {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		parsed = append(parsed, id)
	}
	return parsed
}

func parseID(raw, label string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: invalid %s", models.ErrInvalidInput, label)
	}
	return id, nil
}

// hydrate builds the joined row for one conversation. The repo methods are
// three indexed reads rather than one per-thread query loop.
func (s *chatService) hydrate(ctx context.Context, conversation models.Conversation, viewerID uuid.UUID) (models.ConversationSummary, error) {
	participants, err := s.repo.GetParticipants(ctx, conversation.ID)
	if err != nil {
		return models.ConversationSummary{}, err
	}
	lastMessage, err := s.repo.LastMessageFor(ctx, conversation.ID)
	if err != nil {
		return models.ConversationSummary{}, err
	}
	unread, err := s.repo.UnreadCountFor(ctx, conversation.ID, viewerID)
	if err != nil {
		return models.ConversationSummary{}, err
	}

	return models.ConversationSummary{
		Conversation: conversation,
		Participants: participants,
		LastMessage:  lastMessage,
		UnreadCount:  unread,
	}, nil
}

// requireParticipant is the membership gate every read and write runs first. A
// non-member gets the same error whether the thread exists or not, so the
// service cannot be used to probe for conversation ids.
func (s *chatService) requireParticipant(ctx context.Context, conversationID, viewerID uuid.UUID) (models.Conversation, error) {
	conversation, err := s.repo.GetConversation(ctx, conversationID)
	if err != nil {
		if errors.Is(err, models.ErrConversationNotFound) {
			return models.Conversation{}, models.ErrNotParticipant
		}
		return models.Conversation{}, err
	}

	isMember, err := s.repo.IsParticipant(ctx, conversationID, viewerID)
	if err != nil {
		return models.Conversation{}, err
	}
	if !isMember {
		return models.Conversation{}, models.ErrNotParticipant
	}
	return *conversation, nil
}

func (s *chatService) memberIDs(ctx context.Context, conversationID uuid.UUID) ([]uuid.UUID, error) {
	participants, err := s.repo.GetParticipants(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(participants))
	for _, participant := range participants {
		ids = append(ids, participant.UserID)
	}
	return ids, nil
}

func (s *chatService) CreateConversation(ctx context.Context, creatorID string, participantIDs []string, title string) (*models.ConversationSummary, error) {
	creator, err := parseID(creatorID, "creator id")
	if err != nil {
		return nil, err
	}
	members := parseUUIDs(participantIDs)
	title = strings.TrimSpace(title)

	var conversation *models.Conversation
	switch {
	case len(members) == 1 && title == "":
		// One other member and no title is the direct-thread case: reuse the
		// existing conversation instead of opening a second one.
		if members[0] == creator {
			return nil, models.ErrCannotModerateSelf
		}
		conversation, err = s.repo.CreateDirectConversation(ctx, creator, members[0])
	case len(members) == 0:
		return nil, fmt.Errorf("%w: a group needs at least one other member", models.ErrInvalidInput)
	default:
		if utf8.RuneCountInString(title) > maxGroupTitle {
			return nil, fmt.Errorf("%w: group title must be at most %d characters", models.ErrInvalidInput, maxGroupTitle)
		}
		if len(members) > maxGroupMembers-1 {
			return nil, fmt.Errorf("%w: a group holds at most %d members", models.ErrInvalidInput, maxGroupMembers)
		}
		for _, member := range members {
			if member == creator {
				return nil, models.ErrCannotModerateSelf
			}
		}
		conversation, err = s.repo.CreateGroupConversation(ctx, creator, title, members)
	}
	if err != nil {
		return nil, err
	}

	meta, err := s.hydrate(ctx, *conversation, creator)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

func (s *chatService) GetConversation(ctx context.Context, conversationID, viewerID string) (*models.ConversationSummary, error) {
	id, err := parseID(conversationID, "conversation id")
	if err != nil {
		return nil, err
	}
	viewer, err := parseID(viewerID, "viewer id")
	if err != nil {
		return nil, err
	}

	conversation, err := s.requireParticipant(ctx, id, viewer)
	if err != nil {
		return nil, err
	}

	meta, err := s.hydrate(ctx, conversation, viewer)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

func (s *chatService) ListConversations(
	ctx context.Context,
	viewerID string,
	limit int32,
	cursor string,
	excludedUserIDs []string,
) (*models.InboxPage, error) {
	viewer, err := parseID(viewerID, "viewer id")
	if err != nil {
		return nil, err
	}
	excluded := parseUUIDs(excludedUserIDs)

	fetch := int(limit)
	if fetch <= 0 {
		fetch = defaultPageSize
	}
	if fetch > maxPageSize {
		fetch = maxPageSize
	}

	position, err := models.DecodeConversationCursor(cursor)
	if err != nil {
		return nil, err
	}

	// One extra row so has_more needs no COUNT.
	rows, err := s.repo.ListConversations(ctx, viewer, position, fetch+1, excluded)
	if err != nil {
		return nil, fmt.Errorf("failed to load inbox: %w", err)
	}

	page := &models.InboxPage{Conversations: rows}
	if len(rows) > fetch {
		page.Conversations = rows[:fetch]
		page.HasMore = true
		last := page.Conversations[len(page.Conversations)-1].Conversation
		page.NextCursor = models.EncodeConversationCursor(last.LastMessageAt, last.ID)
	}

	// The badge covers every thread, not just this page, and after the same
	// block exclusions so the two numbers cannot disagree.
	total, err := s.repo.TotalUnread(ctx, viewer, excluded)
	if err != nil {
		return nil, fmt.Errorf("failed to count unread: %w", err)
	}
	page.TotalUnread = total
	return page, nil
}

// TotalUnread counts every unread message across the viewer's threads, minus
// the ones the gateway asked to hide for a block.
func (s *chatService) TotalUnread(ctx context.Context, viewerID string, excluded []uuid.UUID) (int, error) {
	viewer, err := parseID(viewerID, "viewer id")
	if err != nil {
		return 0, err
	}
	return s.repo.TotalUnread(ctx, viewer, excluded)
}

func (s *chatService) SendMessage(ctx context.Context, conversationID, senderID, content string) (*models.Message, error) {
	id, err := parseID(conversationID, "conversation id")
	if err != nil {
		return nil, err
	}
	sender, err := parseID(senderID, "sender id")
	if err != nil {
		return nil, err
	}

	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: message content is required", models.ErrInvalidInput)
	}
	if utf8.RuneCountInString(trimmed) > maxMessageLength {
		return nil, fmt.Errorf("%w: message must be at most %d characters", models.ErrInvalidInput, maxMessageLength)
	}

	isMember, err := s.repo.IsParticipant(ctx, id, sender)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, models.ErrNotParticipant
	}

	message, err := s.repo.SendMessage(ctx, id, sender, trimmed)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Sending is the final "I am done typing" signal, whatever the last
	// keystroke claimed.
	if s.presence != nil {
		s.presence.ClearSenderTyping(ctx, id, sender)
	}

	if s.publisher != nil {
		members, err := s.memberIDs(ctx, id)
		if err == nil {
			_ = s.publisher.PublishMessageCreated(ctx, members, map[string]any{
				"conversation_id": id.String(),
				"message_id":      message.ID.String(),
				"sender_id":       sender.String(),
				"content":         trimmed,
				"created_at":      strconv.FormatInt(message.CreatedAt.UnixMilli(), 10),
			})
		}
	}

	return message, nil
}

func (s *chatService) GetMessages(ctx context.Context, conversationID, viewerID string, limit int32, cursor string) (*models.MessagePage, error) {
	id, err := parseID(conversationID, "conversation id")
	if err != nil {
		return nil, err
	}
	viewer, err := parseID(viewerID, "viewer id")
	if err != nil {
		return nil, err
	}

	isMember, err := s.repo.IsParticipant(ctx, id, viewer)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, models.ErrNotParticipant
	}

	fetch := int(limit)
	if fetch <= 0 {
		fetch = defaultPageSize
	}
	if fetch > maxPageSize {
		fetch = maxPageSize
	}

	position, err := models.DecodeMessageCursor(cursor)
	if err != nil {
		return nil, err
	}

	messages, err := s.repo.GetMessages(ctx, id, viewer, position, fetch+1)
	if err != nil {
		return nil, fmt.Errorf("failed to load messages: %w", err)
	}

	page := &models.MessagePage{Messages: messages}
	if len(messages) > fetch {
		page.Messages = messages[:fetch]
		page.HasMore = true
		last := page.Messages[len(page.Messages)-1]
		page.NextCursor = models.EncodeMessageCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

func (s *chatService) DeleteMessage(ctx context.Context, messageID, senderID string) (*models.Message, error) {
	id, err := parseID(messageID, "message id")
	if err != nil {
		return nil, err
	}
	sender, err := parseID(senderID, "sender id")
	if err != nil {
		return nil, err
	}

	message, err := s.repo.DeleteMessage(ctx, id, sender)
	if err != nil {
		return nil, err
	}

	// The other members learn their conversation now holds a tombstone, so a
	// client can drop the row without refetching the page.
	if s.publisher != nil {
		members, err := s.memberIDs(ctx, message.ConversationID)
		if err == nil {
			recipients := make([]uuid.UUID, 0, len(members))
			for _, member := range members {
				if member != sender {
					recipients = append(recipients, member)
				}
			}
			_ = s.publisher.PublishMessageDeleted(ctx, recipients, message.ConversationID, message.ID)
		}
	}
	return message, nil
}

func (s *chatService) MarkRead(ctx context.Context, conversationID, userID string) error {
	id, err := parseID(conversationID, "conversation id")
	if err != nil {
		return err
	}
	viewer, err := parseID(userID, "user id")
	if err != nil {
		return err
	}

	isMember, err := s.repo.IsParticipant(ctx, id, viewer)
	if err != nil {
		return err
	}
	if !isMember {
		return models.ErrNotParticipant
	}

	if _, err := s.repo.MarkRead(ctx, id, viewer); err != nil {
		return err
	}

	// The other members learn they were read, which is what retires their
	// "not yet seen" affordance without a second round trip.
	if s.publisher != nil {
		members, err := s.memberIDs(ctx, id)
		if err == nil {
			recipients := make([]uuid.UUID, 0, len(members))
			for _, member := range members {
				if member != viewer {
					recipients = append(recipients, member)
				}
			}
			_ = s.publisher.PublishRead(ctx, recipients, id, viewer)
		}
	}
	return nil
}

func (s *chatService) SetTyping(ctx context.Context, conversationID, userID string, isTyping bool) ([]string, error) {
	id, err := parseID(conversationID, "conversation id")
	if err != nil {
		return nil, err
	}
	viewer, err := parseID(userID, "user id")
	if err != nil {
		return nil, err
	}

	isMember, err := s.repo.IsParticipant(ctx, id, viewer)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, models.ErrNotParticipant
	}

	if s.presence == nil {
		return nil, nil
	}

	var typing []uuid.UUID
	if isTyping {
		typing, err = s.presence.StartTyping(ctx, id, viewer)
	} else {
		if err := s.presence.StopTyping(ctx, id, viewer); err != nil {
			return nil, err
		}
		typing, err = s.presence.TypingMembers(ctx, id, viewer)
	}
	if err != nil {
		return nil, err
	}

	results := make([]string, 0, len(typing))
	for _, member := range typing {
		results = append(results, member.String())
	}
	return results, nil
}

func (s *chatService) Heartbeat(ctx context.Context, userID string) error {
	viewer, err := parseID(userID, "user id")
	if err != nil {
		return err
	}
	if s.presence == nil {
		return nil
	}
	return s.presence.Touch(ctx, viewer)
}

func (s *chatService) GetPresence(ctx context.Context, userIDs []string) (map[string]int64, error) {
	if s.presence == nil {
		return map[string]int64{}, nil
	}
	return s.presence.Online(ctx, parseUUIDs(userIDs))
}
