package handler

import (
	"context"
	"errors"
	"time"

	"github.com/chetansachan/socialflow/chat-service/internal/models"
	"github.com/chetansachan/socialflow/chat-service/internal/services"
	chatpb "github.com/chetansachan/socialflow/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ChatHandler struct {
	chatpb.UnimplementedChatServiceServer
	chatService services.ChatService
}

func NewChatHandler(chatService services.ChatService) *ChatHandler {
	return &ChatHandler{chatService: chatService}
}

// chatStatus maps domain errors onto the codes the gateway keys on. Membership
// failures are NotFound rather than PermissionDenied: telling a caller the
// thread exists but is not theirs leaks the id.
func chatStatus(err error) error {
	switch {
	case errors.Is(err, models.ErrNotParticipant):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, models.ErrConversationNotFound), errors.Is(err, models.ErrMessageNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, models.ErrInvalidInput), errors.Is(err, models.ErrCannotModerateSelf):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, models.ErrUserNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func conversationTypeToProto(value string) chatpb.ConversationType {
	if value == models.ConversationGroup {
		return chatpb.ConversationType_CONVERSATION_TYPE_GROUP
	}
	return chatpb.ConversationType_CONVERSATION_TYPE_DIRECT
}

func toProtoConversation(meta models.ConversationSummary) *chatpb.Conversation {
	conversation := &chatpb.Conversation{
		Id:          meta.Conversation.ID.String(),
		Type:        conversationTypeToProto(meta.Conversation.Type),
		Title:       meta.Conversation.Title,
		CreatedBy:   meta.Conversation.CreatedBy.String(),
		UnreadCount: int32(meta.UnreadCount),
		CreatedAt:   meta.Conversation.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   meta.Conversation.UpdatedAt.Format(time.RFC3339),
	}

	for _, participant := range meta.Participants {
		p := &chatpb.Participant{UserId: participant.UserID.String()}
		if participant.LastReadAt != nil {
			p.LastReadAt = participant.LastReadAt.Format(time.RFC3339)
		}
		conversation.Participants = append(conversation.Participants, p)
	}

	if meta.LastMessage != nil {
		conversation.LastMessage = toProtoMessage(meta.LastMessage)
	}
	return conversation
}

func toProtoMessage(message *models.Message) *chatpb.Message {
	if message == nil {
		return nil
	}
	return &chatpb.Message{
		Id:             message.ID.String(),
		ConversationId: message.ConversationID.String(),
		SenderId:       message.SenderID.String(),
		Content:        message.Content,
		CreatedAt:      message.CreatedAt.Format(time.RFC3339),
		IsDeleted:      message.DeletedAt != nil,
	}
}

func (h *ChatHandler) CreateConversation(ctx context.Context, req *chatpb.CreateConversationRequest) (*chatpb.ConversationResponse, error) {
	meta, err := h.chatService.CreateConversation(ctx, req.GetCreatorId(), req.GetParticipantIds(), req.GetTitle())
	if err != nil {
		return nil, chatStatus(err)
	}
	return &chatpb.ConversationResponse{Conversation: toProtoConversation(*meta)}, nil
}

func (h *ChatHandler) GetConversation(ctx context.Context, req *chatpb.GetConversationRequest) (*chatpb.ConversationResponse, error) {
	meta, err := h.chatService.GetConversation(ctx, req.GetId(), req.GetViewerId())
	if err != nil {
		return nil, chatStatus(err)
	}
	return &chatpb.ConversationResponse{Conversation: toProtoConversation(*meta)}, nil
}

func (h *ChatHandler) ListConversations(ctx context.Context, req *chatpb.ListConversationsRequest) (*chatpb.ListConversationsResponse, error) {
	page, err := h.chatService.ListConversations(ctx, req.GetViewerId(), req.GetLimit(), req.GetCursor(), req.GetExcludeUserIds())
	if err != nil {
		return nil, chatStatus(err)
	}

	conversations := make([]*chatpb.Conversation, 0, len(page.Conversations))
	for _, meta := range page.Conversations {
		conversations = append(conversations, toProtoConversation(meta))
	}

	return &chatpb.ListConversationsResponse{
		Conversations: conversations,
		NextCursor:    page.NextCursor,
		HasMore:       page.HasMore,
		TotalUnread:   int32(page.TotalUnread),
	}, nil
}

func (h *ChatHandler) SendMessage(ctx context.Context, req *chatpb.SendMessageRequest) (*chatpb.SendMessageResponse, error) {
	message, err := h.chatService.SendMessage(ctx, req.GetConversationId(), req.GetSenderId(), req.GetContent())
	if err != nil {
		return nil, chatStatus(err)
	}
	return &chatpb.SendMessageResponse{Message: toProtoMessage(message)}, nil
}

func (h *ChatHandler) GetMessages(ctx context.Context, req *chatpb.GetMessagesRequest) (*chatpb.GetMessagesResponse, error) {
	page, err := h.chatService.GetMessages(ctx, req.GetConversationId(), req.GetViewerId(), req.GetLimit(), req.GetCursor())
	if err != nil {
		return nil, chatStatus(err)
	}

	messages := make([]*chatpb.Message, 0, len(page.Messages))
	for i := range page.Messages {
		messages = append(messages, toProtoMessage(&page.Messages[i]))
	}

	return &chatpb.GetMessagesResponse{
		Messages:   messages,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}, nil
}

func (h *ChatHandler) MarkRead(ctx context.Context, req *chatpb.MarkReadRequest) (*chatpb.MarkReadResponse, error) {
	if err := h.chatService.MarkRead(ctx, req.GetConversationId(), req.GetUserId()); err != nil {
		return nil, chatStatus(err)
	}
	return &chatpb.MarkReadResponse{Success: true, LastReadAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

func (h *ChatHandler) DeleteMessage(ctx context.Context, req *chatpb.DeleteMessageRequest) (*chatpb.DeleteMessageResponse, error) {
	if _, err := h.chatService.DeleteMessage(ctx, req.GetMessageId(), req.GetSenderId()); err != nil {
		return nil, chatStatus(err)
	}

	return &chatpb.DeleteMessageResponse{Message: "message deleted"}, nil
}

func (h *ChatHandler) Typing(ctx context.Context, req *chatpb.TypingRequest) (*chatpb.TypingResponse, error) {
	typing, err := h.chatService.SetTyping(ctx, req.GetConversationId(), req.GetUserId(), req.GetIsTyping())
	if err != nil {
		return nil, chatStatus(err)
	}
	return &chatpb.TypingResponse{TypingUserIds: typing}, nil
}

func (h *ChatHandler) Heartbeat(ctx context.Context, req *chatpb.HeartbeatRequest) (*chatpb.PresenceResponse, error) {
	if err := h.chatService.Heartbeat(ctx, req.GetUserId()); err != nil {
		return nil, chatStatus(err)
	}
	// Echo the sender's own presence so a client can confirm the round trip.
	presence, err := h.chatService.GetPresence(ctx, []string{req.GetUserId()})
	if err != nil {
		return nil, chatStatus(err)
	}
	return &chatpb.PresenceResponse{Presence: presence}, nil
}

func (h *ChatHandler) GetPresence(ctx context.Context, req *chatpb.GetPresenceRequest) (*chatpb.PresenceResponse, error) {
	presence, err := h.chatService.GetPresence(ctx, req.GetUserIds())
	if err != nil {
		return nil, chatStatus(err)
	}
	return &chatpb.PresenceResponse{Presence: presence}, nil
}
