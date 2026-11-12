package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chetansachan/socialflow/api-gateway/auth"
	"github.com/chetansachan/socialflow/api-gateway/graph/dataloader"
	"github.com/chetansachan/socialflow/api-gateway/graph/model"
	socialflowpb "github.com/chetansachan/socialflow/proto"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const (
	defaultFeedPageSize         = 20
	maxFeedPageSize             = 50
	defaultCommentPageSize      = 20
	maxCommentPageSize          = 50
	defaultSearchPageSize       = 20
	maxSearchPageSize           = 50
	defaultReportPageSize       = 20
	maxReportPageSize           = 100
	defaultConversationPageSize = 20
	maxConversationPageSize     = 50
	defaultMessagePageSize      = 50
	maxMessagePageSize          = 100
	defaultNotificationPageSize = 20
	maxNotificationPageSize     = 50
)

func toModelUser(user *socialflowpb.User) *model.User {
	if user == nil {
		return nil
	}
	return &model.User{
		ID:        user.GetId(),
		Username:  user.GetUsername(),
		Name:      user.GetName(),
		Email:     user.GetEmail(),
		IsPublic:  user.GetIsPublic(),
		CreatedAt: user.GetCreatedAt(),
		UpdatedAt: user.GetUpdatedAt(),
	}
}

func toModelPost(post *socialflowpb.Post) *model.Post {
	if post == nil {
		return nil
	}
	return &model.Post{
		ID:        post.GetId(),
		AuthorID:  post.GetAuthorId(),
		Content:   post.GetContent(),
		MediaURL:  post.GetMediaUrl(),
		IsPublic:  post.GetIsPublic(),
		LikeCount: post.GetLikeCount(),
		CreatedAt: post.GetCreatedAt(),
		UpdatedAt: post.GetUpdatedAt(),
	}
}

// toModelComment maps one comment node. Author and HasLiked stay zero here and
// are filled by their field resolvers through the request-scoped dataloaders.
func toModelComment(comment *socialflowpb.Comment) *model.Comment {
	if comment == nil {
		return nil
	}

	var parentID *string
	if comment.GetParentId() != "" {
		id := comment.GetParentId()
		parentID = &id
	}

	return &model.Comment{
		ID:         comment.GetId(),
		PostID:     comment.GetPostId(),
		AuthorID:   comment.GetAuthorId(),
		ParentID:   parentID,
		Content:    comment.GetContent(),
		LikeCount:  comment.GetLikeCount(),
		ReplyCount: comment.GetReplyCount(),
		Depth:      comment.GetDepth(),
		IsDeleted:  comment.GetIsDeleted(),
		CreatedAt:  comment.GetCreatedAt(),
		UpdatedAt:  comment.GetUpdatedAt(),
	}
}

func toModelFeedPost(post *socialflowpb.FeedPost) *model.FeedPost {
	if post == nil {
		return nil
	}
	return &model.FeedPost{
		ID:        post.GetId(),
		AuthorID:  post.GetAuthorId(),
		Content:   post.GetContent(),
		MediaURL:  post.GetMediaUrl(),
		IsPublic:  post.GetIsPublic(),
		LikeCount: post.GetLikeCount(),
		CreatedAt: post.GetCreatedAt(),
	}
}

func toModelNotificationActor(actor *socialflowpb.NotificationActor) *model.NotificationActor {
	if actor == nil {
		return nil
	}
	return &model.NotificationActor{
		ID:          actor.GetId(),
		Username:    actor.GetUsername(),
		DisplayName: actor.GetDisplayName(),
		AvatarURL:   actor.GetAvatarUrl(),
	}
}

func toModelNotification(item *socialflowpb.NotificationItem) *model.NotificationItem {
	if item == nil {
		return nil
	}

	actors := make([]*model.NotificationActor, 0, len(item.GetActors()))
	for _, actor := range item.GetActors() {
		actors = append(actors, toModelNotificationActor(actor))
	}

	return &model.NotificationItem{
		ID:               item.GetId(),
		Type:             notificationTypeFromProto(item.GetType()),
		EntityID:         item.GetEntityId(),
		EntityType:       item.GetEntityType(),
		Actors:           actors,
		TotalActorsCount: item.GetTotalActorsCount(),
		Title:            item.GetTitle(),
		ContentPreview:   optionalString(item.GetContentPreview()),
		ActionURL:        item.GetActionUrl(),
		IsRead:           item.GetIsRead(),
		CreatedAt:        item.GetCreatedAt(),
	}
}

func toModelNotificationSettings(settings *socialflowpb.NotificationSettings) *model.NotificationSettings {
	if settings == nil {
		return &model.NotificationSettings{}
	}
	return &model.NotificationSettings{
		InappLikes:          settings.GetInappLikes(),
		InappComments:       settings.GetInappComments(),
		InappFollows:        settings.GetInappFollows(),
		InappMentions:       settings.GetInappMentions(),
		PushEnabled:         settings.GetPushEnabled(),
		PushLikes:           settings.GetPushLikes(),
		PushComments:        settings.GetPushComments(),
		PushFollows:         settings.GetPushFollows(),
		PushMentions:        settings.GetPushMentions(),
		EmailEnabled:        settings.GetEmailEnabled(),
		EmailWeeklyDigest:   settings.GetEmailWeeklyDigest(),
		EmailSecurityAlerts: settings.GetEmailSecurityAlerts(),
		QuietHoursEnabled:   settings.GetQuietHoursEnabled(),
		QuietHoursStart:     settings.GetQuietHoursStart(),
		QuietHoursEnd:       settings.GetQuietHoursEnd(),
		Timezone:            settings.GetTimezone(),
	}
}

func notificationTypeFromProto(value socialflowpb.NotificationType) model.NotificationType {
	switch value {
	case socialflowpb.NotificationType_NOTIFICATION_TYPE_FOLLOW:
		return model.NotificationTypeFollow
	case socialflowpb.NotificationType_NOTIFICATION_TYPE_POST_LIKE:
		return model.NotificationTypePostLike
	case socialflowpb.NotificationType_NOTIFICATION_TYPE_COMMENT:
		return model.NotificationTypeComment
	case socialflowpb.NotificationType_NOTIFICATION_TYPE_COMMENT_REPLY:
		return model.NotificationTypeCommentReply
	case socialflowpb.NotificationType_NOTIFICATION_TYPE_MENTION:
		return model.NotificationTypeMention
	case socialflowpb.NotificationType_NOTIFICATION_TYPE_CREATOR_POST:
		return model.NotificationTypeCreatorPost
	case socialflowpb.NotificationType_NOTIFICATION_TYPE_SYSTEM_ALERT:
		return model.NotificationTypeSystemAlert
	default:
		// New producer categories should not break an inbox page; surface them
		// as a system alert until the gateway schema learns the new value.
		return model.NotificationTypeSystemAlert
	}
}

func notificationTypeFromString(value string) model.NotificationType {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimPrefix(normalized, "notification_type_")
	normalized = strings.ReplaceAll(normalized, "-", "_")

	switch normalized {
	case "follow":
		return model.NotificationTypeFollow
	case "post_like", "like":
		return model.NotificationTypePostLike
	case "comment":
		return model.NotificationTypeComment
	case "comment_reply", "reply":
		return model.NotificationTypeCommentReply
	case "mention":
		return model.NotificationTypeMention
	case "creator_post":
		return model.NotificationTypeCreatorPost
	default:
		return model.NotificationTypeSystemAlert
	}
}

func notificationTypeToProto(value *model.NotificationType) socialflowpb.NotificationType {
	if value == nil {
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED
	}
	switch *value {
	case model.NotificationTypeFollow:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_FOLLOW
	case model.NotificationTypePostLike:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_POST_LIKE
	case model.NotificationTypeComment:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_COMMENT
	case model.NotificationTypeCommentReply:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_COMMENT_REPLY
	case model.NotificationTypeMention:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_MENTION
	case model.NotificationTypeCreatorPost:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_CREATOR_POST
	case model.NotificationTypeSystemAlert:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_SYSTEM_ALERT
	default:
		return socialflowpb.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED
	}
}

func notificationEntityType(value *model.NotificationType) string {
	if value == nil {
		return "system"
	}
	switch *value {
	case model.NotificationTypeFollow:
		return "user"
	case model.NotificationTypeComment, model.NotificationTypeCommentReply, model.NotificationTypeMention:
		return "comment"
	case model.NotificationTypeSystemAlert:
		return "system"
	default:
		return "post"
	}
}

func toProtoSettings(settings model.NotificationSettings) *socialflowpb.NotificationSettings {
	return &socialflowpb.NotificationSettings{
		InappLikes:          settings.InappLikes,
		InappComments:       settings.InappComments,
		InappFollows:        settings.InappFollows,
		InappMentions:       settings.InappMentions,
		PushEnabled:         settings.PushEnabled,
		PushLikes:           settings.PushLikes,
		PushComments:        settings.PushComments,
		PushFollows:         settings.PushFollows,
		PushMentions:        settings.PushMentions,
		EmailEnabled:        settings.EmailEnabled,
		EmailWeeklyDigest:   settings.EmailWeeklyDigest,
		EmailSecurityAlerts: settings.EmailSecurityAlerts,
		QuietHoursEnabled:   settings.QuietHoursEnabled,
		QuietHoursStart:     settings.QuietHoursStart,
		QuietHoursEnd:       settings.QuietHoursEnd,
		Timezone:            settings.Timezone,
	}
}

// applySettings overlays only the fields the client supplied, preserving every
// stored preference the request did not mention.
func applySettings(current *model.NotificationSettings, input model.UpdateNotificationSettingsInput) model.NotificationSettings {
	next := *current
	if input.InappLikes != nil {
		next.InappLikes = *input.InappLikes
	}
	if input.InappComments != nil {
		next.InappComments = *input.InappComments
	}
	if input.InappFollows != nil {
		next.InappFollows = *input.InappFollows
	}
	if input.InappMentions != nil {
		next.InappMentions = *input.InappMentions
	}
	if input.PushEnabled != nil {
		next.PushEnabled = *input.PushEnabled
	}
	if input.PushLikes != nil {
		next.PushLikes = *input.PushLikes
	}
	if input.PushComments != nil {
		next.PushComments = *input.PushComments
	}
	if input.PushFollows != nil {
		next.PushFollows = *input.PushFollows
	}
	if input.PushMentions != nil {
		next.PushMentions = *input.PushMentions
	}
	if input.EmailEnabled != nil {
		next.EmailEnabled = *input.EmailEnabled
	}
	if input.EmailWeeklyDigest != nil {
		next.EmailWeeklyDigest = *input.EmailWeeklyDigest
	}
	if input.EmailSecurityAlerts != nil {
		next.EmailSecurityAlerts = *input.EmailSecurityAlerts
	}
	if input.QuietHoursEnabled != nil {
		next.QuietHoursEnabled = *input.QuietHoursEnabled
	}
	if input.QuietHoursStart != nil {
		next.QuietHoursStart = *input.QuietHoursStart
	}
	if input.QuietHoursEnd != nil {
		next.QuietHoursEnd = *input.QuietHoursEnd
	}
	if input.Timezone != nil {
		next.Timezone = *input.Timezone
	}
	return next
}

// requireViewer returns the authenticated caller and a GraphQL-facing error
// when the request is anonymous.
func requireViewer(ctx context.Context) (auth.Principal, error) {
	return auth.Require(ctx)
}

// requireSelf rejects requests that try to act on behalf of another user.
// Authorization is decided from the verified token, never from client input.
func requireSelf(principal auth.Principal, requestedUserID string) error {
	if strings.TrimSpace(requestedUserID) == "" {
		return nil
	}
	if requestedUserID != principal.UserID {
		return &gqlerror.Error{
			Message:    "cannot act on behalf of another user",
			Extensions: map[string]any{"code": "FORBIDDEN"},
		}
	}
	return nil
}

func clampPageSize(requested *int32, fallback, maximum int32) int32 {
	if requested == nil || *requested <= 0 {
		return fallback
	}
	if *requested > maximum {
		return maximum
	}
	return *requested
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// loadAuthor resolves a post author through the request-scoped user loader so
// a feed page issues one batch lookup instead of one RPC per post.
func loadAuthor(ctx context.Context, authorID string) (*model.User, error) {
	loaders := dataloader.FromContext(ctx)
	if loaders == nil || loaders.UserByID == nil {
		return nil, fmt.Errorf("dataloader middleware is not configured")
	}
	user, err := loaders.RequireUser(ctx, authorID)
	if err != nil {
		return nil, err
	}
	return toModelUser(user), nil
}

// loadLikeState returns the viewer's like state for one post. Anonymous
// callers always see false rather than an authorization error: like state is
// public metadata, not private data.
func loadLikeState(ctx context.Context, postID string) (bool, error) {
	principal, ok := auth.FromContext(ctx)
	if !ok || strings.TrimSpace(principal.UserID) == "" {
		return false, nil
	}

	loaders := dataloader.FromContext(ctx)
	if loaders == nil || loaders.LikeState == nil {
		return false, fmt.Errorf("dataloader middleware is not configured")
	}

	liked, err := loaders.LikeState.Load(ctx, dataloader.LikeKey{
		ViewerID: principal.UserID,
		PostID:   postID,
	})()
	if err != nil {
		return false, err
	}
	return liked, nil
}

// loadCommentLikeState returns the viewer's like state for one comment.
// Anonymous callers see false, matching post behavior.
func loadCommentLikeState(ctx context.Context, commentID string) (bool, error) {
	principal, ok := auth.FromContext(ctx)
	if !ok || strings.TrimSpace(principal.UserID) == "" {
		return false, nil
	}

	loaders := dataloader.FromContext(ctx)
	if loaders == nil || loaders.CommentLikeState == nil {
		return false, fmt.Errorf("dataloader middleware is not configured")
	}

	liked, err := loaders.CommentLikeState.Load(ctx, dataloader.CommentLikeKey{
		ViewerID:  principal.UserID,
		CommentID: commentID,
	})()
	if err != nil {
		return false, err
	}
	return liked, nil
}

// requireAdmin rejects non-admin callers before they can read or change the
// moderation queue. Role defaults to "user" when the token omits it.
func requireAdmin(principal auth.Principal) error {
	if principal.Role == "admin" {
		return nil
	}
	return &gqlerror.Error{
		Message:    "administrator role required",
		Extensions: map[string]any{"code": "FORBIDDEN"},
	}
}

// reportTargetTypeToProto maps the GraphQL enum onto the gRPC contract.
func reportTargetTypeToProto(value model.ReportTargetType) socialflowpb.ReportTargetType {
	switch value {
	case model.ReportTargetTypePost:
		return socialflowpb.ReportTargetType_REPORT_TARGET_TYPE_POST
	case model.ReportTargetTypeComment:
		return socialflowpb.ReportTargetType_REPORT_TARGET_TYPE_COMMENT
	case model.ReportTargetTypeUser:
		return socialflowpb.ReportTargetType_REPORT_TARGET_TYPE_USER
	default:
		return socialflowpb.ReportTargetType_REPORT_TARGET_TYPE_UNSPECIFIED
	}
}

func reportTargetTypeFromProto(value socialflowpb.ReportTargetType) model.ReportTargetType {
	switch value {
	case socialflowpb.ReportTargetType_REPORT_TARGET_TYPE_POST:
		return model.ReportTargetTypePost
	case socialflowpb.ReportTargetType_REPORT_TARGET_TYPE_COMMENT:
		return model.ReportTargetTypeComment
	case socialflowpb.ReportTargetType_REPORT_TARGET_TYPE_USER:
		return model.ReportTargetTypeUser
	default:
		return ""
	}
}

func reportReasonToProto(value model.ReportReason) socialflowpb.ReportReason {
	switch value {
	case model.ReportReasonSpam:
		return socialflowpb.ReportReason_REPORT_REASON_SPAM
	case model.ReportReasonHarassment:
		return socialflowpb.ReportReason_REPORT_REASON_HARASSMENT
	case model.ReportReasonHate:
		return socialflowpb.ReportReason_REPORT_REASON_HATE
	case model.ReportReasonViolence:
		return socialflowpb.ReportReason_REPORT_REASON_VIOLENCE
	case model.ReportReasonNsfw:
		return socialflowpb.ReportReason_REPORT_REASON_NSFW
	case model.ReportReasonImpersonation:
		return socialflowpb.ReportReason_REPORT_REASON_IMPERSONATION
	case model.ReportReasonSelfHarm:
		return socialflowpb.ReportReason_REPORT_REASON_SELF_HARM
	case model.ReportReasonOther:
		return socialflowpb.ReportReason_REPORT_REASON_OTHER
	default:
		return socialflowpb.ReportReason_REPORT_REASON_UNSPECIFIED
	}
}

func reportReasonFromProto(value socialflowpb.ReportReason) model.ReportReason {
	switch value {
	case socialflowpb.ReportReason_REPORT_REASON_SPAM:
		return model.ReportReasonSpam
	case socialflowpb.ReportReason_REPORT_REASON_HARASSMENT:
		return model.ReportReasonHarassment
	case socialflowpb.ReportReason_REPORT_REASON_HATE:
		return model.ReportReasonHate
	case socialflowpb.ReportReason_REPORT_REASON_VIOLENCE:
		return model.ReportReasonViolence
	case socialflowpb.ReportReason_REPORT_REASON_NSFW:
		return model.ReportReasonNsfw
	case socialflowpb.ReportReason_REPORT_REASON_IMPERSONATION:
		return model.ReportReasonImpersonation
	case socialflowpb.ReportReason_REPORT_REASON_SELF_HARM:
		return model.ReportReasonSelfHarm
	case socialflowpb.ReportReason_REPORT_REASON_OTHER:
		return model.ReportReasonOther
	default:
		return ""
	}
}

func reportStatusToProto(value model.ReportStatus) socialflowpb.ReportStatus {
	switch value {
	case model.ReportStatusPending:
		return socialflowpb.ReportStatus_REPORT_STATUS_PENDING
	case model.ReportStatusReviewing:
		return socialflowpb.ReportStatus_REPORT_STATUS_REVIEWING
	case model.ReportStatusActioned:
		return socialflowpb.ReportStatus_REPORT_STATUS_ACTIONED
	case model.ReportStatusDismissed:
		return socialflowpb.ReportStatus_REPORT_STATUS_DISMISSED
	default:
		return socialflowpb.ReportStatus_REPORT_STATUS_UNSPECIFIED
	}
}

func reportStatusFromProto(value socialflowpb.ReportStatus) model.ReportStatus {
	switch value {
	case socialflowpb.ReportStatus_REPORT_STATUS_PENDING:
		return model.ReportStatusPending
	case socialflowpb.ReportStatus_REPORT_STATUS_REVIEWING:
		return model.ReportStatusReviewing
	case socialflowpb.ReportStatus_REPORT_STATUS_ACTIONED:
		return model.ReportStatusActioned
	case socialflowpb.ReportStatus_REPORT_STATUS_DISMISSED:
		return model.ReportStatusDismissed
	default:
		return ""
	}
}

// toModelReport maps a moderation report. The optional review metadata stays
// nil when the report has not been reviewed so clients can tell open from
// resolved without inspecting the status too.
func toModelReport(report *socialflowpb.Report) *model.Report {
	if report == nil {
		return nil
	}

	mapped := &model.Report{
		ID:         report.GetId(),
		ReporterID: report.GetReporterId(),
		TargetType: reportTargetTypeFromProto(report.GetTargetType()),
		TargetID:   report.GetTargetId(),
		Reason:     reportReasonFromProto(report.GetReason()),
		Details:    report.GetDetails(),
		Status:     reportStatusFromProto(report.GetStatus()),
		CreatedAt:  report.GetCreatedAt(),
		ReviewNote: report.GetReviewNote(),
	}
	if reviewedAt := report.GetReviewedAt(); reviewedAt != "" {
		mapped.ReviewedAt = &reviewedAt
	}
	if reviewerID := report.GetReviewerId(); reviewerID != "" {
		mapped.ReviewerID = &reviewerID
	}
	return mapped
}

// requirePostOwner rejects a mutation aimed at somebody else's post. The
// owner is read from post-service, never from client input.
func requirePostOwner(principal auth.Principal, authorID string) error {
	if authorID == principal.UserID {
		return nil
	}
	return &gqlerror.Error{
		Message:    "cannot modify another user's post",
		Extensions: map[string]any{"code": "FORBIDDEN"},
	}
}

// requirePostOwnership loads the post first so delete and archive cannot be
// used to probe or mutate another user's content.
func (r *Resolver) requirePostOwnership(ctx context.Context, principal auth.Principal, postID string) error {
	res, err := r.PostClient.GetPostById(ctx, &socialflowpb.PostByIdRequest{Id: postID})
	if err != nil {
		return err
	}
	return requirePostOwner(principal, res.GetPost().GetAuthorId())
}

// blockedUserIDs returns the ids the viewer asked to hide. Only the viewer's own
// block list is passed on: moderation-service never reveals who blocked the
// viewer, so the set is deliberately one-directional here and chat-service
// simply drops the threads that contain any of them.
func (r *Resolver) blockedUserIDs(ctx context.Context, principal auth.Principal) []string {
	if r.ModerationClient == nil {
		return nil
	}
	res, err := r.ModerationClient.ListBlockedUsers(ctx, &socialflowpb.ListRelationshipsRequest{
		UserId: principal.UserID,
	})
	if err != nil {
		// A block list that cannot be loaded degrades to "no blocks" rather
		// than failing the chat read, which is the safe direction for a
		// visibility feature the viewer already saw.
		return nil
	}
	return res.GetUserIds()
}

// realtimeChannel is the Redis Pub/Sub channel notification-service publishes
// one user's frames to.
func realtimeChannel(userID string) string {
	return "notify:" + strings.TrimSpace(userID)
}

// chatChannel is the Redis Pub/Sub channel chat-service publishes one user's
// frames to. It is separate from the notification channel so a client can
// subscribe to one stream without paying for the other's frames.
func chatChannel(userID string) string {
	return "chat:" + strings.TrimSpace(userID)
}

// chatFrame mirrors the envelope chat-service publishes. Data is kept raw so a
// new frame type does not need a schema change to reach the relay.
type chatFrame struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type chatEventData struct {
	ConversationID string `json:"conversation_id"`
	SenderID       string `json:"sender_id"`
	UserID         string `json:"user_id"`
	IsTyping       bool   `json:"is_typing"`
	CreatedAt      string `json:"created_at"`

	// A message frame carries the body inline so a client can render without
	// refetching the thread it is already looking at.
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
	IsDeleted bool   `json:"is_deleted"`
}

// chatEventFromFrame converts one chat frame into the GraphQL type. Frames this
// version does not understand are dropped rather than surfaced as half-empty
// events.
func chatEventFromFrame(payload []byte) (*model.ChatEvent, error) {
	var frame chatFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		return nil, fmt.Errorf("decode chat frame: %w", err)
	}

	switch frame.Type {
	case "message.created", "message.deleted", "read", "typing":
	default:
		return nil, fmt.Errorf("unsupported chat frame type %q", frame.Type)
	}

	var data chatEventData
	if err := json.Unmarshal(frame.Data, &data); err != nil {
		return nil, fmt.Errorf("decode chat frame payload: %w", err)
	}

	event := &model.ChatEvent{
		Type:           frame.Type,
		ConversationID: optionalString(data.ConversationID),
		SenderID:       optionalString(data.SenderID),
		IsTyping:       data.IsTyping,
		TypingUserIds:  []string{},
	}

	senderID := data.SenderID
	if senderID == "" {
		senderID = data.UserID
	}
	if senderID != "" {
		event.SenderID = &[]string{senderID}[0]
	}

	if frame.Type == "message.created" || frame.Type == "message.deleted" {
		if data.MessageID == "" {
			return nil, fmt.Errorf("chat %s frame has no message id", frame.Type)
		}
		event.Message = &model.Message{
			ID:             data.MessageID,
			ConversationID: data.ConversationID,
			SenderID:       senderID,
			Content:        data.Content,
			CreatedAt:      data.CreatedAt,
			IsDeleted:      frame.Type == "message.deleted",
		}
	}

	return event, nil
}

// toModelParticipant maps one membership row. user stays nil and is filled by
// the field resolver through the request-scoped loader.
func toModelParticipant(participant *socialflowpb.Participant) *model.ConversationParticipant {
	if participant == nil {
		return nil
	}
	mapped := &model.ConversationParticipant{UserID: participant.GetUserId()}
	if lastReadAt := participant.GetLastReadAt(); lastReadAt != "" {
		mapped.LastReadAt = &lastReadAt
	}
	return mapped
}

func toModelMessage(message *socialflowpb.Message) *model.Message {
	if message == nil {
		return nil
	}
	return &model.Message{
		ID:             message.GetId(),
		ConversationID: message.GetConversationId(),
		SenderID:       message.GetSenderId(),
		Content:        message.GetContent(),
		CreatedAt:      message.GetCreatedAt(),
		IsDeleted:      message.GetIsDeleted(),
	}
}

// conversationTypeFromProto returns the short scheme-independent name, not the
// generated enum's full constant, so clients see "DIRECT" and "GROUP".
func conversationTypeFromProto(value socialflowpb.ConversationType) string {
	if value == socialflowpb.ConversationType_CONVERSATION_TYPE_GROUP {
		return "GROUP"
	}
	return "DIRECT"
}

func toModelConversation(conversation *socialflowpb.Conversation) *model.Conversation {
	if conversation == nil {
		return nil
	}

	participants := make([]*model.ConversationParticipant, 0, len(conversation.GetParticipants()))
	for _, participant := range conversation.GetParticipants() {
		participants = append(participants, toModelParticipant(participant))
	}

	return &model.Conversation{
		ID:           conversation.GetId(),
		Type:         conversationTypeFromProto(conversation.GetType()),
		Title:        conversation.GetTitle(),
		CreatedBy:    conversation.GetCreatedBy(),
		Participants: participants,
		LastMessage:  toModelMessage(conversation.GetLastMessage()),
		UnreadCount:  conversation.GetUnreadCount(),
		CreatedAt:    conversation.GetCreatedAt(),
		UpdatedAt:    conversation.GetUpdatedAt(),
	}
}

// hydrateConversation resolves the participant profiles and the last message's
// sender through the request-scoped user loader. The chat-service payload only
// carries ids, while `ConversationParticipant.user` and `Message.sender` are
// non-null in the schema — without this, a valid thread serialises as a GraphQL
// error instead of a conversation.
func hydrateConversation(ctx context.Context, conversation *model.Conversation) (*model.Conversation, error) {
	if conversation == nil {
		return nil, nil
	}
	for _, participant := range conversation.Participants {
		if participant == nil || participant.User != nil {
			continue
		}
		// A blank id is left for the field resolver to reject on its own, so one
		// incomplete row does not fail the whole page.
		if strings.TrimSpace(participant.UserID) == "" {
			continue
		}
		user, err := loadAuthor(ctx, participant.UserID)
		if err != nil {
			return nil, err
		}
		participant.User = user
	}
	if conversation.LastMessage != nil {
		hydrated, err := hydrateMessage(ctx, conversation.LastMessage)
		if err != nil {
			return nil, err
		}
		conversation.LastMessage = hydrated
	}
	return conversation, nil
}

func hydrateMessage(ctx context.Context, message *model.Message) (*model.Message, error) {
	if message == nil || message.Sender != nil {
		return message, nil
	}
	// Same as above: an absent sender id keeps the message and lets the `sender`
	// field resolver report the specific failure, rather than taking the whole
	// transcript down with it.
	if strings.TrimSpace(message.SenderID) == "" {
		return message, nil
	}
	sender, err := loadAuthor(ctx, message.SenderID)
	if err != nil {
		return nil, err
	}
	message.Sender = sender
	return message, nil
}

// realtimeFrame mirrors the WebSocket envelope published by notification-service.
type realtimeFrame struct {
	Event string                   `json:"event"`
	Data  realtimeNotificationData `json:"data"`
}

type realtimeNotificationData struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	Title            string          `json:"title"`
	ContentPreview   string          `json:"content_preview"`
	ActionURL        string          `json:"action_url"`
	Actors           []realtimeActor `json:"actors"`
	TotalActorsCount int32           `json:"total_actors_count"`
	UnreadCount      int32           `json:"unread_count"`
	CreatedAt        string          `json:"created_at"`
}

type realtimeActor struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

// notificationItemFromFrame converts a realtime frame into the GraphQL type.
// The frame intentionally omits entity id, entity type, and read state, so they
// are derived here rather than forcing the dispatcher to send another query.
func notificationItemFromFrame(payload []byte) (*model.NotificationItem, error) {
	var frame realtimeFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		return nil, fmt.Errorf("decode realtime notification: %w", err)
	}
	if strings.TrimSpace(frame.Data.ID) == "" {
		return nil, fmt.Errorf("realtime notification is missing an id")
	}

	itemType := notificationTypeFromString(frame.Data.Type)
	actors := make([]*model.NotificationActor, 0, len(frame.Data.Actors))
	for _, actor := range frame.Data.Actors {
		actors = append(actors, &model.NotificationActor{
			ID:          actor.ID,
			Username:    actor.Username,
			DisplayName: actor.DisplayName,
			AvatarURL:   actor.AvatarURL,
		})
	}

	// A follow notification points at the follower's profile, so the single
	// actor is also the entity. Other categories reference content the compact
	// frame does not name; clients refetch the inbox row before navigating.
	entityID := ""
	if itemType == model.NotificationTypeFollow && len(actors) == 1 {
		entityID = actors[0].ID
	}

	return &model.NotificationItem{
		ID:               frame.Data.ID,
		Type:             itemType,
		EntityID:         entityID,
		EntityType:       notificationEntityType(&itemType),
		Actors:           actors,
		TotalActorsCount: frame.Data.TotalActorsCount,
		Title:            frame.Data.Title,
		ContentPreview:   optionalString(frame.Data.ContentPreview),
		ActionURL:        frame.Data.ActionURL,
		IsRead:           false,
		CreatedAt:        frame.Data.CreatedAt,
	}, nil
}
