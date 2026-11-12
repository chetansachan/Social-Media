package graph

import (
	"context"
	"testing"

	"github.com/chetansachan/socialflow/api-gateway/graph/dataloader"
	"github.com/chetansachan/socialflow/api-gateway/graph/model"
	socialflowpb "github.com/chetansachan/socialflow/proto"
	"google.golang.org/grpc"
)

type fakeChatClient struct {
	socialflowpb.ChatServiceClient

	createRequest *socialflowpb.CreateConversationRequest
	sendRequest   *socialflowpb.SendMessageRequest
	markReadReq   *socialflowpb.MarkReadRequest
	typingReq     *socialflowpb.TypingRequest
	deleteReq     *socialflowpb.DeleteMessageRequest
	listReq       *socialflowpb.ListConversationsRequest
	messagesReq   *socialflowpb.GetMessagesRequest
	conversation  *socialflowpb.ConversationResponse
	getReq        *socialflowpb.GetConversationRequest
	presenceReq   *socialflowpb.GetPresenceRequest

	conversationMeta *socialflowpb.Conversation
	listResponse     *socialflowpb.ListConversationsResponse
	message          *socialflowpb.Message
	messageResponse  *socialflowpb.GetMessagesResponse
	success          bool
	typing           []string
	presence         map[string]int64
	excluded         []string
}

func (c *fakeChatClient) CreateConversation(ctx context.Context, req *socialflowpb.CreateConversationRequest, _ ...grpc.CallOption) (*socialflowpb.ConversationResponse, error) {
	c.createRequest = req
	return &socialflowpb.ConversationResponse{Conversation: c.conversationMeta}, nil
}

func (c *fakeChatClient) GetConversation(ctx context.Context, req *socialflowpb.GetConversationRequest, _ ...grpc.CallOption) (*socialflowpb.ConversationResponse, error) {
	c.getReq = req
	return c.conversation, nil
}

func (c *fakeChatClient) ListConversations(ctx context.Context, req *socialflowpb.ListConversationsRequest, _ ...grpc.CallOption) (*socialflowpb.ListConversationsResponse, error) {
	c.listReq = req
	return c.listResponse, nil
}

func (c *fakeChatClient) SendMessage(ctx context.Context, req *socialflowpb.SendMessageRequest, _ ...grpc.CallOption) (*socialflowpb.SendMessageResponse, error) {
	c.sendRequest = req
	return &socialflowpb.SendMessageResponse{Message: c.message}, nil
}

func (c *fakeChatClient) GetMessages(ctx context.Context, req *socialflowpb.GetMessagesRequest, _ ...grpc.CallOption) (*socialflowpb.GetMessagesResponse, error) {
	c.messagesReq = req
	return c.messageResponse, nil
}

func (c *fakeChatClient) MarkRead(ctx context.Context, req *socialflowpb.MarkReadRequest, _ ...grpc.CallOption) (*socialflowpb.MarkReadResponse, error) {
	c.markReadReq = req
	return &socialflowpb.MarkReadResponse{Success: c.success}, nil
}

func (c *fakeChatClient) Typing(ctx context.Context, req *socialflowpb.TypingRequest, _ ...grpc.CallOption) (*socialflowpb.TypingResponse, error) {
	c.typingReq = req
	return &socialflowpb.TypingResponse{TypingUserIds: c.typing}, nil
}

func (c *fakeChatClient) DeleteMessage(ctx context.Context, req *socialflowpb.DeleteMessageRequest, _ ...grpc.CallOption) (*socialflowpb.DeleteMessageResponse, error) {
	c.deleteReq = req
	return &socialflowpb.DeleteMessageResponse{Message: "message deleted"}, nil
}

func (c *fakeChatClient) GetPresence(ctx context.Context, req *socialflowpb.GetPresenceRequest, _ ...grpc.CallOption) (*socialflowpb.PresenceResponse, error) {
	c.presenceReq = req
	return &socialflowpb.PresenceResponse{Presence: c.presence}, nil
}

// chatLoaderContext builds a viewer context with the request-scoped user loader
// installed, exactly as the dataloader middleware installs it in production.
//
// The chat resolvers hydrate participant and sender ids through that loader
// rather than leaving the schema's non-null `user`/`sender` fields unresolved,
// so any test that reaches them needs loaders in the context.
func chatLoaderContext(userIDs ...string) context.Context {
	users := make(map[string]*socialflowpb.User, len(userIDs))
	for _, id := range userIDs {
		users[id] = &socialflowpb.User{Id: id, Username: id, Name: id}
	}
	loaders := dataloader.New(&batchUserClient{users: users}, nil, nil)
	return dataloader.WithLoaders(viewerContext("u1"), loaders)
}

func sampleConversation() *socialflowpb.Conversation {
	return &socialflowpb.Conversation{
		Id:        "c1",
		Type:      socialflowpb.ConversationType_CONVERSATION_TYPE_DIRECT,
		CreatedBy: "u1",
		Participants: []*socialflowpb.Participant{
			{UserId: "u1", LastReadAt: "2026-01-01T00:00:00Z"},
			{UserId: "u2"},
		},
		LastMessage: &socialflowpb.Message{Id: "m1", SenderId: "u2", Content: "hi"},
		UnreadCount: 1,
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-01-01T00:00:00Z",
	}
}

func sampleMessages() []*socialflowpb.Message {
	return []*socialflowpb.Message{
		{Id: "m1", SenderId: "u2", Content: "one"},
		{Id: "m2", SenderId: "u1", Content: "", IsDeleted: true},
	}
}

func TestCreateConversationScopesCreator(t *testing.T) {
	client := &fakeChatClient{conversationMeta: sampleConversation()}
	resolver := &mutationResolver{&Resolver{ChatClient: client}}

	title := "Team"
	res, err := resolver.CreateConversation(chatLoaderContext("u1", "u2"), model.CreateConversationInput{
		ParticipantIds: []string{"u2", "u3"},
		Title:          &title,
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if client.createRequest.GetCreatorId() != "u1" {
		t.Fatalf("creator = %q, want the authenticated caller", client.createRequest.GetCreatorId())
	}
	if len(client.createRequest.GetParticipantIds()) != 2 {
		t.Fatalf("participants = %v", client.createRequest.GetParticipantIds())
	}
	if res.Type != "DIRECT" || len(res.Participants) != 2 {
		t.Fatalf("conversation = %+v", res)
	}
	if res.Participants[0].LastReadAt == nil || *res.Participants[0].LastReadAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("participant 0 = %+v", res.Participants[0])
	}
	if res.Participants[1].LastReadAt != nil {
		t.Fatal("an unread member should have no watermark")
	}
	if res.LastMessage == nil || res.LastMessage.Content != "hi" {
		t.Fatalf("last message = %+v", res.LastMessage)
	}
	// `user` and `sender` are non-null in the schema, so they must be hydrated
	// here. Leaving them nil would serialise as a GraphQL error at runtime.
	for i, participant := range res.Participants {
		if participant.User == nil || participant.User.ID != participant.UserID {
			t.Fatalf("participant %d was not hydrated: %+v", i, participant)
		}
	}
	if res.LastMessage.Sender == nil || res.LastMessage.Sender.ID != "u2" {
		t.Fatalf("last message sender was not hydrated: %+v", res.LastMessage.Sender)
	}
}

func TestSendMessageScopesSender(t *testing.T) {
	client := &fakeChatClient{message: &socialflowpb.Message{Id: "m1", Content: "hi"}}
	resolver := &mutationResolver{&Resolver{ChatClient: client}}

	res, err := resolver.SendMessage(chatLoaderContext("u1"), model.SendMessageInput{
		ConversationID: "c1",
		Content:        "hi",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if client.sendRequest.GetSenderId() != "u1" || client.sendRequest.GetContent() != "hi" {
		t.Fatalf("send request = %+v", client.sendRequest)
	}
	if res.ID != "m1" {
		t.Fatalf("message = %+v", res)
	}
}

func TestChatMutationsRequireAuthentication(t *testing.T) {
	resolver := &mutationResolver{&Resolver{ChatClient: &fakeChatClient{}}}
	ctx := context.Background()

	if _, err := resolver.CreateConversation(ctx, model.CreateConversationInput{ParticipantIds: []string{"u2"}}); err == nil {
		t.Fatal("anonymous create should fail")
	}
	if _, err := resolver.SendMessage(ctx, model.SendMessageInput{ConversationID: "c1", Content: "hi"}); err == nil {
		t.Fatal("anonymous send should fail")
	}
	if _, err := resolver.MarkConversationRead(ctx, "c1"); err == nil {
		t.Fatal("anonymous mark-read should fail")
	}
	if _, err := resolver.SetTyping(ctx, model.TypingInput{ConversationID: "c1", IsTyping: true}); err == nil {
		t.Fatal("anonymous typing should fail")
	}
	if _, err := resolver.DeleteMessage(ctx, "m1"); err == nil {
		t.Fatal("anonymous delete should fail")
	}
}

func TestMarkReadAndTypingScopeToCaller(t *testing.T) {
	client := &fakeChatClient{success: true, typing: []string{"u2"}}
	resolver := &mutationResolver{&Resolver{ChatClient: client}}

	ok, err := resolver.MarkConversationRead(viewerContext("u1"), "c1")
	if err != nil || !ok {
		t.Fatalf("MarkConversationRead = %v, %v", ok, err)
	}
	if client.markReadReq.GetUserId() != "u1" || client.markReadReq.GetConversationId() != "c1" {
		t.Fatalf("mark-read request = %+v", client.markReadReq)
	}

	typing, err := resolver.SetTyping(viewerContext("u1"), model.TypingInput{ConversationID: "c1", IsTyping: true})
	if err != nil {
		t.Fatalf("SetTyping: %v", err)
	}
	if client.typingReq.GetUserId() != "u1" || !client.typingReq.GetIsTyping() {
		t.Fatalf("typing request = %+v", client.typingReq)
	}
	if len(typing) != 1 || typing[0] != "u2" {
		t.Fatalf("typing = %v", typing)
	}
}

func TestSetTypingNeverReturnsNil(t *testing.T) {
	resolver := &mutationResolver{&Resolver{ChatClient: &fakeChatClient{}}}

	typing, err := resolver.SetTyping(viewerContext("u1"), model.TypingInput{ConversationID: "c1"})
	if err != nil {
		t.Fatalf("SetTyping: %v", err)
	}
	if typing == nil {
		t.Fatal("a nil typists list should serialise as [] for clients")
	}
}

func TestDeleteMessageScopesSender(t *testing.T) {
	client := &fakeChatClient{}
	resolver := &mutationResolver{&Resolver{ChatClient: client}}

	done, err := resolver.DeleteMessage(viewerContext("u1"), "m1")
	if err != nil || !done {
		t.Fatalf("DeleteMessage = %v, %v", done, err)
	}
	if client.deleteReq.GetSenderId() != "u1" || client.deleteReq.GetMessageId() != "m1" {
		t.Fatalf("delete request = %+v", client.deleteReq)
	}
}

func TestMyConversationsScopesToCallerAndClamps(t *testing.T) {
	client := &fakeChatClient{listResponse: &socialflowpb.ListConversationsResponse{
		Conversations: []*socialflowpb.Conversation{sampleConversation()},
		NextCursor:    "cursor-2",
		HasMore:       true,
		TotalUnread:   9,
	}}
	resolver := &queryResolver{&Resolver{ChatClient: client}}

	limit := int32(1000)
	res, err := resolver.MyConversations(chatLoaderContext("u1", "u2"), &limit, nil)
	if err != nil {
		t.Fatalf("MyConversations: %v", err)
	}

	if client.listReq.GetViewerId() != "u1" {
		t.Fatalf("viewer = %q, want the authenticated caller", client.listReq.GetViewerId())
	}
	if client.listReq.GetLimit() != maxConversationPageSize {
		t.Fatalf("limit = %d, want %d", client.listReq.GetLimit(), maxConversationPageSize)
	}
	if len(client.listReq.GetExcludeUserIds()) != 0 {
		t.Fatalf("excluded = %v, want none when moderation is not configured", client.listReq.GetExcludeUserIds())
	}
	if len(res.Conversations) != 1 || !res.HasMore || res.TotalUnread != 9 {
		t.Fatalf("page = %+v", res)
	}
	if res.NextCursor == nil || *res.NextCursor != "cursor-2" {
		t.Fatalf("cursor = %v", res.NextCursor)
	}
}

func TestMyConversationsDefaultsPageSize(t *testing.T) {
	client := &fakeChatClient{listResponse: &socialflowpb.ListConversationsResponse{}}
	resolver := &queryResolver{&Resolver{ChatClient: client}}

	if _, err := resolver.MyConversations(viewerContext("u1"), nil, nil); err != nil {
		t.Fatalf("MyConversations: %v", err)
	}
	if client.listReq.GetLimit() != defaultConversationPageSize {
		t.Fatalf("limit = %d, want %d", client.listReq.GetLimit(), defaultConversationPageSize)
	}
}

func TestGetConversationAndMessagesScopeToCaller(t *testing.T) {
	client := &fakeChatClient{
		conversation: &socialflowpb.ConversationResponse{Conversation: sampleConversation()},
		messageResponse: &socialflowpb.GetMessagesResponse{
			Messages:   sampleMessages(),
			NextCursor: "next",
			HasMore:    true,
		},
	}
	resolver := &queryResolver{&Resolver{ChatClient: client}}

	conversation, err := resolver.GetConversation(chatLoaderContext("u1", "u2"), "c1")
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if client.getReq.GetViewerId() != "u1" || client.getReq.GetId() != "c1" {
		t.Fatalf("conversation request = %+v", client.getReq)
	}
	if conversation.ID != "c1" {
		t.Fatalf("conversation = %+v", conversation)
	}

	limit := int32(1000)
	page, err := resolver.GetMessages(chatLoaderContext("u1", "u2"), "c1", &limit, nil)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if client.messagesReq.GetViewerId() != "u1" {
		t.Fatalf("messages viewer = %q", client.messagesReq.GetViewerId())
	}
	if client.messagesReq.GetLimit() != maxMessagePageSize {
		t.Fatalf("limit = %d, want %d", client.messagesReq.GetLimit(), maxMessagePageSize)
	}
	if len(page.Messages) != 2 || !page.Messages[1].IsDeleted {
		t.Fatalf("page = %+v", page)
	}
	if page.Messages[1].Content != "" {
		t.Fatal("a tombstone should not carry its old body")
	}
}

func TestChatQueriesRequireAuthentication(t *testing.T) {
	resolver := &queryResolver{&Resolver{ChatClient: &fakeChatClient{}}}
	ctx := context.Background()

	if _, err := resolver.MyConversations(ctx, nil, nil); err == nil {
		t.Fatal("anonymous inbox should fail")
	}
	if _, err := resolver.GetConversation(ctx, "c1"); err == nil {
		t.Fatal("anonymous conversation read should fail")
	}
	if _, err := resolver.GetMessages(ctx, "c1", nil, nil); err == nil {
		t.Fatal("anonymous message read should fail")
	}
	if _, err := resolver.GetPresence(ctx, []string{"u1"}); err == nil {
		t.Fatal("anonymous presence should fail")
	}
}

func TestGetPresenceReturnsMap(t *testing.T) {
	client := &fakeChatClient{presence: map[string]int64{"u1": 1700000000000}}
	resolver := &queryResolver{&Resolver{ChatClient: client}}

	presence, err := resolver.GetPresence(viewerContext("u1"), []string{"u1"})
	if err != nil {
		t.Fatalf("GetPresence: %v", err)
	}
	if len(client.presenceReq.GetUserIds()) != 1 {
		t.Fatalf("requested ids = %v", client.presenceReq.GetUserIds())
	}
	if seenAt, ok := presence["u1"].(int64); !ok || seenAt != 1700000000000 {
		t.Fatalf("presence = %#v", presence)
	}
}

func TestChatEventFromFrame(t *testing.T) {
	messageFrame := []byte(`{"type":"message.created","data":{"conversation_id":"c1","sender_id":"u2","message_id":"m1","content":"hi","created_at":"2026-01-01T00:00:00Z"}}`)
	event, err := chatEventFromFrame(messageFrame)
	if err != nil {
		t.Fatalf("decode message frame: %v", err)
	}
	if event.Type != "message.created" || event.Message == nil || event.Message.ID != "m1" {
		t.Fatalf("event = %+v", event)
	}
	if event.ConversationID == nil || *event.ConversationID != "c1" {
		t.Fatalf("conversation = %v", event.ConversationID)
	}
	if event.SenderID == nil || *event.SenderID != "u2" {
		t.Fatalf("sender = %v", event.SenderID)
	}

	typingFrame := []byte(`{"type":"typing","data":{"conversation_id":"c1","user_id":"u2","is_typing":true}}`)
	event, err = chatEventFromFrame(typingFrame)
	if err != nil {
		t.Fatalf("decode typing frame: %v", err)
	}
	if !event.IsTyping || event.Message != nil {
		t.Fatalf("typing event = %+v", event)
	}

	readFrame := []byte(`{"type":"read","data":{"conversation_id":"c1","user_id":"u2"}}`)
	if _, err := chatEventFromFrame(readFrame); err != nil {
		t.Fatalf("decode read frame: %v", err)
	}

	for _, payload := range []string{
		"not-json",
		`{"type":"unknown","data":{}}`,
		`{"type":"message.created","data":{"conversation_id":"c1"}}`,
	} {
		if _, err := chatEventFromFrame([]byte(payload)); err == nil {
			t.Fatalf("payload %q should be rejected", payload)
		}
	}
}
