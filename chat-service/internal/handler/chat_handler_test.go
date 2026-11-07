package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/chat-service/internal/models"
	"github.com/chetansachan/socialflow/chat-service/internal/services"
	chatpb "github.com/chetansachan/socialflow/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeChatService struct {
	err error

	summary  models.ConversationSummary
	inbox    *models.InboxPage
	message  *models.Message
	page     *models.MessagePage
	typing   []string
	presence map[string]int64

	lastParticipantIDs []string
	lastTitle          string
	lastConversationID string
	lastViewerID       string
	lastExcluded       []string
	lastContent        string
	lastIsTyping       bool
}

func (s *fakeChatService) CreateConversation(ctx context.Context, creatorID string, participantIDs []string, title string) (*models.ConversationSummary, error) {
	s.lastParticipantIDs = participantIDs
	s.lastTitle = title
	return &s.summary, s.err
}

func (s *fakeChatService) GetConversation(ctx context.Context, conversationID, viewerID string) (*models.ConversationSummary, error) {
	s.lastConversationID = conversationID
	s.lastViewerID = viewerID
	return &s.summary, s.err
}

func (s *fakeChatService) ListConversations(ctx context.Context, viewerID string, limit int32, cursor string, excludedUserIDs []string) (*models.InboxPage, error) {
	s.lastViewerID = viewerID
	s.lastExcluded = excludedUserIDs
	return s.inbox, s.err
}

func (s *fakeChatService) SendMessage(ctx context.Context, conversationID, senderID, content string) (*models.Message, error) {
	s.lastConversationID = conversationID
	s.lastContent = content
	return s.message, s.err
}

func (s *fakeChatService) GetMessages(ctx context.Context, conversationID, viewerID string, limit int32, cursor string) (*models.MessagePage, error) {
	s.lastConversationID = conversationID
	s.lastViewerID = viewerID
	return s.page, s.err
}

func (s *fakeChatService) DeleteMessage(ctx context.Context, messageID, senderID string) (*models.Message, error) {
	return &models.Message{ID: uuid.New()}, s.err
}

func (s *fakeChatService) MarkRead(ctx context.Context, conversationID, userID string) error {
	s.lastConversationID = conversationID
	return s.err
}

func (s *fakeChatService) TotalUnread(ctx context.Context, viewerID string, excluded []uuid.UUID) (int, error) {
	return 0, s.err
}

func (s *fakeChatService) SetTyping(ctx context.Context, conversationID, userID string, isTyping bool) ([]string, error) {
	s.lastConversationID = conversationID
	s.lastIsTyping = isTyping
	return s.typing, s.err
}

func (s *fakeChatService) Heartbeat(ctx context.Context, userID string) error {
	s.lastViewerID = userID
	return s.err
}

func (s *fakeChatService) GetPresence(ctx context.Context, userIDs []string) (map[string]int64, error) {
	return s.presence, s.err
}

// fakeChatService must satisfy the service interface the handler depends on.
var _ services.ChatService = (*fakeChatService)(nil)

func sampleSummary() models.ConversationSummary {
	return models.ConversationSummary{
		Conversation: models.Conversation{
			Type:      models.ConversationGroup,
			Title:     "Team",
			CreatedBy: uuid.New(),
		},
		Participants: []models.Participant{{UserID: uuid.New()}, {UserID: uuid.New()}},
		LastMessage: &models.Message{
			ID:      uuid.New(),
			Content: "hi",
		},
		UnreadCount: 2,
	}
}

func TestHandlerMapsConversationToProto(t *testing.T) {
	h := NewChatHandler(&fakeChatService{summary: sampleSummary()})

	res, err := h.GetConversation(context.Background(), &chatpb.GetConversationRequest{Id: "c1", ViewerId: "u1"})
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if res.GetConversation().GetType() != chatpb.ConversationType_CONVERSATION_TYPE_GROUP {
		t.Fatalf("type = %v", res.GetConversation().GetType())
	}
	if len(res.GetConversation().GetParticipants()) != 2 {
		t.Fatalf("participants = %d", len(res.GetConversation().GetParticipants()))
	}
	if res.GetConversation().GetLastMessage().GetContent() != "hi" {
		t.Fatalf("last message = %+v", res.GetConversation().GetLastMessage())
	}
	if res.GetConversation().GetUnreadCount() != 2 {
		t.Fatalf("unread = %d", res.GetConversation().GetUnreadCount())
	}
}

func TestHandlerMapsDomainErrors(t *testing.T) {
	cases := []struct {
		err  error
		want codes.Code
	}{
		// A membership failure must read as "not found" so ids cannot be probed.
		{models.ErrNotParticipant, codes.NotFound},
		{models.ErrConversationNotFound, codes.NotFound},
		{models.ErrMessageNotFound, codes.NotFound},
		{models.ErrInvalidInput, codes.InvalidArgument},
		{models.ErrCannotModerateSelf, codes.InvalidArgument},
		{errors.New("connection reset"), codes.Internal},
	}
	for _, tc := range cases {
		h := NewChatHandler(&fakeChatService{err: tc.err})

		_, err := h.GetConversation(context.Background(), &chatpb.GetConversationRequest{Id: "c1", ViewerId: "u1"})
		if status.Code(err) != tc.want {
			t.Fatalf("%v: code = %v, want %v", tc.err, status.Code(err), tc.want)
		}

		_, err = h.SendMessage(context.Background(), &chatpb.SendMessageRequest{ConversationId: "c1", SenderId: "u1", Content: "hi"})
		if status.Code(err) != tc.want {
			t.Fatalf("%v (send): code = %v, want %v", tc.err, status.Code(err), tc.want)
		}
	}
}

func TestHandlerPassesPaginationAndBadge(t *testing.T) {
	h := NewChatHandler(&fakeChatService{inbox: &models.InboxPage{
		Conversations: []models.ConversationSummary{sampleSummary()},
		NextCursor:    "next",
		HasMore:       true,
		TotalUnread:   7,
	}})

	res, err := h.ListConversations(context.Background(), &chatpb.ListConversationsRequest{
		ViewerId:       "u1",
		Limit:          25,
		ExcludeUserIds: []string{"blocked"},
	})
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(res.GetConversations()) != 1 || res.GetNextCursor() != "next" || !res.GetHasMore() {
		t.Fatalf("page = %+v", res)
	}
	if res.GetTotalUnread() != 7 {
		t.Fatalf("total unread = %d, want 7", res.GetTotalUnread())
	}
}

func TestHandlerPassesBlockExclusions(t *testing.T) {
	service := &fakeChatService{inbox: &models.InboxPage{}}
	h := NewChatHandler(service)

	if _, err := h.ListConversations(context.Background(), &chatpb.ListConversationsRequest{
		ViewerId:       "u1",
		ExcludeUserIds: []string{"blocked-1", "blocked-2"},
	}); err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(service.lastExcluded) != 2 || service.lastExcluded[0] != "blocked-1" {
		t.Fatalf("excluded = %v, want both hidden ids", service.lastExcluded)
	}
}

func TestHandlerMessagesPageMapsTombstone(t *testing.T) {
	deletedAt := time.Now().UTC()
	h := NewChatHandler(&fakeChatService{page: &models.MessagePage{
		Messages: []models.Message{
			{ID: uuid.New(), Content: "", DeletedAt: &deletedAt},
			{ID: uuid.New(), Content: "still here"},
		},
	}})

	// A tombstone is an empty body plus is_deleted; the content the row no
	// longer holds is not re-derived here.
	res, err := h.GetMessages(context.Background(), &chatpb.GetMessagesRequest{ConversationId: "c1", ViewerId: "u1"})
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(res.GetMessages()) != 2 {
		t.Fatalf("messages = %d", len(res.GetMessages()))
	}
	if !res.GetMessages()[0].GetIsDeleted() {
		t.Fatal("the first message should map as deleted")
	}
	if res.GetMessages()[1].GetIsDeleted() {
		t.Fatal("the second message should not be marked deleted")
	}
	if res.GetMessages()[1].GetContent() != "still here" {
		t.Fatalf("second message = %q", res.GetMessages()[1].GetContent())
	}
}

func TestHandlerTypingAndPresence(t *testing.T) {
	h := NewChatHandler(&fakeChatService{
		typing:   []string{"u2", "u3"},
		presence: map[string]int64{"u1": 1700000000000},
	})

	res, err := h.Typing(context.Background(), &chatpb.TypingRequest{ConversationId: "c1", UserId: "u1", IsTyping: true})
	if err != nil {
		t.Fatalf("Typing: %v", err)
	}
	if len(res.GetTypingUserIds()) != 2 {
		t.Fatalf("typing = %v", res.GetTypingUserIds())
	}

	presence, err := h.GetPresence(context.Background(), &chatpb.GetPresenceRequest{UserIds: []string{"u1"}})
	if err != nil {
		t.Fatalf("GetPresence: %v", err)
	}
	if presence.GetPresence()["u1"] != 1700000000000 {
		t.Fatalf("presence = %v", presence.GetPresence())
	}

	if _, err := h.Heartbeat(context.Background(), &chatpb.HeartbeatRequest{UserId: "u1"}); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
}
