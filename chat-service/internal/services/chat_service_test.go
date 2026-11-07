package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/chat-service/internal/models"
	"github.com/google/uuid"
)

type fakeChatRepo struct {
	conversations map[uuid.UUID]*models.Conversation
	members       map[uuid.UUID][]uuid.UUID
	messages      map[uuid.UUID][]models.Message
	readWatermark map[uuid.UUID]map[uuid.UUID]time.Time

	createdDirect           bool
	createdGroup            bool
	lastDirectPair          [2]uuid.UUID
	lastGroupTitle          string
	lastGroupIDs            []uuid.UUID
	lastSender              uuid.UUID
	lastContent             string
	lastMarkRead            uuid.UUID
	lastLimit               int
	lastTotalUnreadExcluded int
	deleted                 uuid.UUID
}

func newFakeChatRepo() *fakeChatRepo {
	return &fakeChatRepo{
		conversations: make(map[uuid.UUID]*models.Conversation),
		members:       make(map[uuid.UUID][]uuid.UUID),
		messages:      make(map[uuid.UUID][]models.Message),
		readWatermark: make(map[uuid.UUID]map[uuid.UUID]time.Time),
	}
}

func (r *fakeChatRepo) addMembers(conversationID uuid.UUID, ids ...uuid.UUID) {
	r.members[conversationID] = append(r.members[conversationID], ids...)
}

func (r *fakeChatRepo) CreateDirectConversation(ctx context.Context, creatorID, otherID uuid.UUID) (*models.Conversation, error) {
	r.createdDirect = true
	r.lastDirectPair = [2]uuid.UUID{creatorID, otherID}

	for _, conversation := range r.conversations {
		if conversation.Type != models.ConversationDirect {
			continue
		}
		members := r.members[conversation.ID]
		if len(members) != 2 {
			continue
		}
		if (members[0] == creatorID && members[1] == otherID) ||
			(members[0] == otherID && members[1] == creatorID) {
			return conversation, nil
		}
	}

	return r.storeConversation(models.ConversationDirect, "", creatorID, otherID, uuid.Nil), nil
}

func (r *fakeChatRepo) CreateGroupConversation(ctx context.Context, creatorID uuid.UUID, title string, participantIDs []uuid.UUID) (*models.Conversation, error) {
	r.createdGroup = true
	r.lastGroupTitle = title
	r.lastGroupIDs = participantIDs
	members := append([]uuid.UUID{creatorID}, participantIDs...)
	return r.storeConversation(models.ConversationGroup, title, creatorID, uuid.Nil, members...), nil
}

func (r *fakeChatRepo) storeConversation(kind, title string, creatorID, directOther uuid.UUID, members ...uuid.UUID) *models.Conversation {
	conversation := &models.Conversation{
		ID:        uuid.New(),
		Type:      kind,
		Title:     title,
		CreatedBy: creatorID,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	r.conversations[conversation.ID] = conversation
	if directOther != uuid.Nil {
		r.addMembers(conversation.ID, creatorID, directOther)
	} else {
		r.addMembers(conversation.ID, members...)
	}
	return conversation
}

func (r *fakeChatRepo) GetConversation(ctx context.Context, id uuid.UUID) (*models.Conversation, error) {
	conversation, ok := r.conversations[id]
	if !ok {
		return nil, models.ErrConversationNotFound
	}
	return conversation, nil
}

func (r *fakeChatRepo) IsParticipant(ctx context.Context, conversationID, userID uuid.UUID) (bool, error) {
	for _, member := range r.members[conversationID] {
		if member == userID {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeChatRepo) GetParticipants(ctx context.Context, conversationID uuid.UUID) ([]models.Participant, error) {
	participants := make([]models.Participant, 0, len(r.members[conversationID]))
	for _, member := range r.members[conversationID] {
		participants = append(participants, models.Participant{UserID: member})
	}
	return participants, nil
}

func (r *fakeChatRepo) LastMessageFor(ctx context.Context, conversationID uuid.UUID) (*models.Message, error) {
	messages := r.messages[conversationID]
	if len(messages) == 0 {
		return nil, nil
	}
	last := messages[len(messages)-1]
	return &last, nil
}

func (r *fakeChatRepo) UnreadCountFor(ctx context.Context, conversationID, viewerID uuid.UUID) (int, error) {
	return len(r.messages[conversationID]), nil
}

func (r *fakeChatRepo) ListConversations(ctx context.Context, viewerID uuid.UUID, cursor models.ConversationCursor, limit int, excluded []uuid.UUID) ([]models.ConversationSummary, error) {
	r.lastLimit = limit
	r.lastTotalUnreadExcluded = len(excluded)
	rows := make([]models.ConversationSummary, 0, len(r.conversations))
	for _, conversation := range r.conversations {
		rows = append(rows, models.ConversationSummary{Conversation: *conversation})
	}
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func (r *fakeChatRepo) GetMessages(ctx context.Context, conversationID, viewerID uuid.UUID, cursor models.MessageCursor, limit int) ([]models.Message, error) {
	messages := r.messages[conversationID]
	if limit > 0 && len(messages) > limit {
		messages = messages[:limit]
	}
	return messages, nil
}

func (r *fakeChatRepo) SendMessage(ctx context.Context, conversationID, senderID uuid.UUID, content string) (*models.Message, error) {
	r.lastSender = senderID
	r.lastContent = content
	message := models.Message{
		ID:             uuid.New(),
		ConversationID: conversationID,
		SenderID:       senderID,
		Content:        content,
		CreatedAt:      time.Now().UTC(),
	}
	r.messages[conversationID] = append(r.messages[conversationID], message)
	return &message, nil
}

func (r *fakeChatRepo) DeleteMessage(ctx context.Context, messageID, senderID uuid.UUID) (*models.Message, error) {
	r.deleted = messageID
	for _, messages := range r.messages {
		for i := range messages {
			if messages[i].ID == messageID {
				message := messages[i]
				message.Content = ""
				message.DeletedAt = &message.CreatedAt
				messages[i] = message
				return &message, nil
			}
		}
	}
	return &models.Message{ID: messageID, SenderID: senderID}, nil
}

func (r *fakeChatRepo) MarkRead(ctx context.Context, conversationID, userID uuid.UUID) (*time.Time, error) {
	r.lastMarkRead = conversationID
	if r.readWatermark[conversationID] == nil {
		r.readWatermark[conversationID] = make(map[uuid.UUID]time.Time)
	}
	now := time.Now().UTC()
	r.readWatermark[conversationID][userID] = now
	return &now, nil
}

func (r *fakeChatRepo) TotalUnread(ctx context.Context, viewerID uuid.UUID, excluded []uuid.UUID) (int, error) {
	r.lastTotalUnreadExcluded = len(excluded)
	return len(r.messages), nil
}

func TestCreateConversationRejectsSelf(t *testing.T) {
	repo := newFakeChatRepo()
	service := NewChatService(repo, nil, nil)
	id := uuid.New()

	if _, err := service.CreateConversation(context.Background(), id.String(), []string{id.String()}, ""); !errors.Is(err, models.ErrCannotModerateSelf) {
		t.Fatalf("self conversation err = %v, want ErrCannotModerateSelf", err)
	}

	// A self-member inside a larger group is rejected too.
	other := uuid.New()
	if _, err := service.CreateConversation(context.Background(), id.String(), []string{other.String(), id.String()}, "group"); !errors.Is(err, models.ErrCannotModerateSelf) {
		t.Fatalf("self in group err = %v, want ErrCannotModerateSelf", err)
	}
}

func TestCreateConversationRoutesDirectAndGroup(t *testing.T) {
	repo := newFakeChatRepo()
	service := NewChatService(repo, nil, nil)
	alice, bob := uuid.New(), uuid.New()

	if _, err := service.CreateConversation(context.Background(), alice.String(), []string{bob.String()}, ""); err != nil {
		t.Fatalf("direct create: %v", err)
	}
	if !repo.createdDirect || repo.lastDirectPair != [2]uuid.UUID{alice, bob} {
		t.Fatalf("direct create not routed: pair = %v", repo.lastDirectPair)
	}

	if _, err := service.CreateConversation(context.Background(), alice.String(), []string{bob.String(), uuid.New().String()}, "Team"); err != nil {
		t.Fatalf("group create: %v", err)
	}
	if !repo.createdGroup || repo.lastGroupTitle != "Team" || len(repo.lastGroupIDs) != 2 {
		t.Fatalf("group create not routed: %v/%d members", repo.lastGroupTitle, len(repo.lastGroupIDs))
	}
}

func TestCreateConversationRejectsEmptyGroupAndBadTitle(t *testing.T) {
	service := NewChatService(newFakeChatRepo(), nil, nil)
	alice := uuid.New()

	if _, err := service.CreateConversation(context.Background(), alice.String(), nil, ""); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("no members err = %v, want ErrInvalidInput", err)
	}
	if _, err := service.CreateConversation(context.Background(), alice.String(), []string{uuid.New().String()}, strings.Repeat("t", maxGroupTitle+1)); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("long title err = %v, want ErrInvalidInput", err)
	}
}

func TestSendMessageRejectsNonMembersAndEmptyContent(t *testing.T) {
	repo := newFakeChatRepo()
	service := NewChatService(repo, nil, nil)
	alice := uuid.New()
	conversation := repo.storeConversation(models.ConversationDirect, "", alice, uuid.New())
	stranger := uuid.New()

	if _, err := service.SendMessage(context.Background(), conversation.ID.String(), stranger.String(), "hi"); !errors.Is(err, models.ErrNotParticipant) {
		t.Fatalf("non-member err = %v, want ErrNotParticipant", err)
	}

	for _, content := range []string{"", "   ", strings.Repeat("a", maxMessageLength+1)} {
		if _, err := service.SendMessage(context.Background(), conversation.ID.String(), alice.String(), content); !errors.Is(err, models.ErrInvalidInput) {
			t.Fatalf("content len %d: err = %v, want ErrInvalidInput", len(content), err)
		}
	}
}

func TestSendMessageTrimsAndRecords(t *testing.T) {
	repo := newFakeChatRepo()
	service := NewChatService(repo, nil, nil)
	alice := uuid.New()
	conversation := repo.storeConversation(models.ConversationDirect, "", alice, uuid.New())

	message, err := service.SendMessage(context.Background(), conversation.ID.String(), alice.String(), "  hello  ")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if repo.lastContent != "hello" {
		t.Fatalf("stored content = %q, want the trimmed value", repo.lastContent)
	}
	if message.SenderID != alice {
		t.Fatalf("sender = %s", message.SenderID)
	}
}

func TestMessagesHideFromNonMembers(t *testing.T) {
	repo := newFakeChatRepo()
	service := NewChatService(repo, nil, nil)
	alice := uuid.New()
	conversation := repo.storeConversation(models.ConversationDirect, "", alice, uuid.New())

	if _, err := service.GetMessages(context.Background(), conversation.ID.String(), uuid.New().String(), 0, ""); !errors.Is(err, models.ErrNotParticipant) {
		t.Fatalf("non-member read err = %v, want ErrNotParticipant", err)
	}

	page, err := service.GetMessages(context.Background(), conversation.ID.String(), alice.String(), 0, "")
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if page == nil {
		t.Fatal("page should not be nil")
	}
}

func TestGetConversationUnknownLooksLikeNotParticipant(t *testing.T) {
	service := NewChatService(newFakeChatRepo(), nil, nil)

	_, err := service.GetConversation(context.Background(), uuid.New().String(), uuid.New().String())
	if !errors.Is(err, models.ErrNotParticipant) {
		t.Fatalf("unknown conversation err = %v, want ErrNotParticipant so ids cannot be probed", err)
	}
}

func TestMarkReadRequiresMembership(t *testing.T) {
	repo := newFakeChatRepo()
	service := NewChatService(repo, nil, nil)
	alice := uuid.New()
	conversation := repo.storeConversation(models.ConversationDirect, "", alice, uuid.New())

	if err := service.MarkRead(context.Background(), conversation.ID.String(), uuid.New().String()); !errors.Is(err, models.ErrNotParticipant) {
		t.Fatalf("non-member mark-read err = %v, want ErrNotParticipant", err)
	}
	if err := service.MarkRead(context.Background(), conversation.ID.String(), alice.String()); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if repo.lastMarkRead != conversation.ID {
		t.Fatalf("marked = %s, want %s", repo.lastMarkRead, conversation.ID)
	}
}

func TestListConversationsClampsLimit(t *testing.T) {
	repo := newFakeChatRepo()
	service := NewChatService(repo, nil, nil)
	alice := uuid.New()
	// Enough threads that every page size is distinguishable.
	for i := 0; i < 60; i++ {
		repo.storeConversation(models.ConversationDirect, "", alice, uuid.New())
	}

	for _, tc := range []struct {
		name      string
		requested int32
		want      int
	}{
		{name: "default", requested: 0, want: defaultPageSize},
		{name: "negative", requested: -1, want: defaultPageSize},
		{name: "clamped", requested: 900, want: maxPageSize},
		{name: "explicit", requested: 5, want: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := service.ListConversations(context.Background(), alice.String(), tc.requested, "", nil)
			if err != nil {
				t.Fatalf("ListConversations: %v", err)
			}
			// The repository is asked for one extra row to decide has_more.
			if repo.lastLimit != tc.want+1 {
				t.Fatalf("repository limit = %d, want %d", repo.lastLimit, tc.want+1)
			}
			if len(page.Conversations) != tc.want {
				t.Fatalf("conversations = %d, want %d", len(page.Conversations), tc.want)
			}
			if !page.HasMore || page.NextCursor == "" {
				t.Fatalf("page should report more results: %+v", page)
			}
		})
	}

	if _, err := service.ListConversations(context.Background(), alice.String(), 10, "not-a-cursor", nil); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("bad cursor err = %v, want ErrInvalidInput", err)
	}
}

func TestTypingAndPresenceWithoutRedisAreSilent(t *testing.T) {
	repo := newFakeChatRepo()
	service := NewChatService(repo, nil, nil)
	alice := uuid.New()
	conversation := repo.storeConversation(models.ConversationDirect, "", alice, uuid.New())

	typing, err := service.SetTyping(context.Background(), conversation.ID.String(), alice.String(), true)
	if err != nil {
		t.Fatalf("SetTyping without redis: %v", err)
	}
	if len(typing) != 0 {
		t.Fatalf("typing = %v, want nobody without a presence store", typing)
	}

	if err := service.Heartbeat(context.Background(), alice.String()); err != nil {
		t.Fatalf("Heartbeat without redis: %v", err)
	}
	presence, err := service.GetPresence(context.Background(), []string{alice.String()})
	if err != nil {
		t.Fatalf("GetPresence without redis: %v", err)
	}
	if len(presence) != 0 {
		t.Fatalf("presence = %v, want nobody offline without redis", presence)
	}
}

func TestTypingRejectsNonMembers(t *testing.T) {
	repo := newFakeChatRepo()
	service := NewChatService(repo, nil, nil)
	conversation := repo.storeConversation(models.ConversationDirect, "", uuid.New(), uuid.New())

	if _, err := service.SetTyping(context.Background(), conversation.ID.String(), uuid.New().String(), true); !errors.Is(err, models.ErrNotParticipant) {
		t.Fatalf("typing err = %v, want ErrNotParticipant", err)
	}
}

// fakePublisher records the frames the service would relay, so tests can assert
// that realtime fan-out actually happens rather than only that the flag was set.
type fakePublisher struct {
	typingMembers      []uuid.UUID
	typingConversation uuid.UUID
	typingUser         uuid.UUID
	typingIsTyping     bool

	readMembers      []uuid.UUID
	readConversation uuid.UUID
	readUser         uuid.UUID

	createdMembers []uuid.UUID
	deletedMembers []uuid.UUID
	deletedMessage uuid.UUID
}

func (p *fakePublisher) PublishMessageCreated(ctx context.Context, members []uuid.UUID, payload map[string]any) error {
	p.createdMembers = members
	return nil
}

func (p *fakePublisher) PublishMessageDeleted(ctx context.Context, members []uuid.UUID, conversationID, messageID uuid.UUID) error {
	p.deletedMembers = members
	p.deletedMessage = messageID
	return nil
}

func (p *fakePublisher) PublishTyping(ctx context.Context, members []uuid.UUID, conversationID, userID uuid.UUID, isTyping bool) error {
	p.typingMembers = members
	p.typingConversation = conversationID
	p.typingUser = userID
	p.typingIsTyping = isTyping
	return nil
}

func (p *fakePublisher) PublishRead(ctx context.Context, members []uuid.UUID, conversationID, userID uuid.UUID) error {
	p.readMembers = members
	p.readConversation = conversationID
	p.readUser = userID
	return nil
}

func TestTypingIsRelayedToOtherMembers(t *testing.T) {
	repo := newFakeChatRepo()
	alice, bob := uuid.New(), uuid.New()
	conversation := repo.storeConversation(models.ConversationDirect, "", alice, bob)

	service := NewChatService(repo, nil, nil)
	service.(*chatService).publisher = &fakePublisher{}

	if _, err := service.SetTyping(context.Background(), conversation.ID.String(), alice.String(), true); err != nil {
		t.Fatalf("SetTyping: %v", err)
	}

	publisher := service.(*chatService).publisher.(*fakePublisher)
	if len(publisher.typingMembers) != 1 || publisher.typingMembers[0] != bob {
		t.Fatalf("typing fanned out to %v, want [bob]", publisher.typingMembers)
	}
	if publisher.typingUser != alice || !publisher.typingIsTyping {
		t.Fatalf("frame = %s/%v", publisher.typingUser, publisher.typingIsTyping)
	}
}

func TestMarkReadAndDeleteAreRelayedToOtherMembers(t *testing.T) {
	repo := newFakeChatRepo()
	alice, bob := uuid.New(), uuid.New()
	conversation := repo.storeConversation(models.ConversationDirect, "", alice, bob)

	service := NewChatService(repo, nil, nil)
	publisher := &fakePublisher{}
	service.(*chatService).publisher = publisher

	if err := service.MarkRead(context.Background(), conversation.ID.String(), alice.String()); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if len(publisher.readMembers) != 1 || publisher.readMembers[0] != bob {
		t.Fatalf("read fanned out to %v, want [bob]", publisher.readMembers)
	}

	// Send first so the fake has a row to look the conversation up from.
	message, err := service.SendMessage(context.Background(), conversation.ID.String(), alice.String(), "hello")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if _, err := service.DeleteMessage(context.Background(), message.ID.String(), alice.String()); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if len(publisher.deletedMembers) != 1 || publisher.deletedMembers[0] != bob {
		t.Fatalf("deletion fanned out to %v, want [bob]", publisher.deletedMembers)
	}
	if publisher.deletedMessage != message.ID {
		t.Fatalf("deleted frame named %s, want %s", publisher.deletedMessage, message.ID)
	}
}
