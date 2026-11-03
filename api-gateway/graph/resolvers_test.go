package graph

import (
	"context"
	"strings"
	"testing"

	"github.com/chetansachan/socialflow/api-gateway/auth"
	"github.com/chetansachan/socialflow/api-gateway/graph/model"
	socialflowpb "github.com/chetansachan/socialflow/proto"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"google.golang.org/grpc"
)

type fakeUserClient struct {
	socialflowpb.UserServiceClient

	updateRequests []*socialflowpb.UpdateUserRequest
}

func (c *fakeUserClient) UpdateUser(ctx context.Context, req *socialflowpb.UpdateUserRequest, _ ...grpc.CallOption) (*socialflowpb.UpdateUserResponse, error) {
	c.updateRequests = append(c.updateRequests, req)
	return &socialflowpb.UpdateUserResponse{User: &socialflowpb.User{
		Id:       req.GetId(),
		Username: req.GetUsername(),
		Name:     req.GetName(),
		Email:    req.GetEmail(),
	}}, nil
}

type fakePostClient struct {
	socialflowpb.PostServiceClient

	posts       map[string]*socialflowpb.Post
	deleted     []string
	archived    []string
	likeStates  map[string]bool
	getPostErrs error

	signatureRequest *socialflowpb.GetUploadSignatureRequest
}

func (c *fakePostClient) GetPostById(ctx context.Context, req *socialflowpb.PostByIdRequest, _ ...grpc.CallOption) (*socialflowpb.PostByIdResponse, error) {
	if c.getPostErrs != nil {
		return nil, c.getPostErrs
	}
	post, ok := c.posts[req.GetId()]
	if !ok {
		return nil, gqlerror.Errorf("post not found")
	}
	return &socialflowpb.PostByIdResponse{Post: post}, nil
}

func (c *fakePostClient) DeletePost(ctx context.Context, req *socialflowpb.DeletePostRequest, _ ...grpc.CallOption) (*socialflowpb.DeletePostResponse, error) {
	c.deleted = append(c.deleted, req.GetPostId())
	return &socialflowpb.DeletePostResponse{Message: "deleted"}, nil
}

func (c *fakePostClient) MovePostToArchived(ctx context.Context, req *socialflowpb.MovePostToArchivedRequest, _ ...grpc.CallOption) (*socialflowpb.MovePostToArchivedResponse, error) {
	c.archived = append(c.archived, req.GetPostId())
	return &socialflowpb.MovePostToArchivedResponse{Message: "archived"}, nil
}

func (c *fakePostClient) GetLikeStates(ctx context.Context, req *socialflowpb.GetLikeStatesRequest, _ ...grpc.CallOption) (*socialflowpb.GetLikeStatesResponse, error) {
	liked := make(map[string]bool, len(req.GetPostIds()))
	for _, postID := range req.GetPostIds() {
		liked[postID] = c.likeStates[postID]
	}
	return &socialflowpb.GetLikeStatesResponse{Liked: liked}, nil
}

func (c *fakePostClient) GetUploadSignature(ctx context.Context, req *socialflowpb.GetUploadSignatureRequest, _ ...grpc.CallOption) (*socialflowpb.GetUploadSignatureResponse, error) {
	c.signatureRequest = req
	return &socialflowpb.GetUploadSignatureResponse{
		CloudName: "demo-cloud",
		ApiKey:    "demo-key",
		Timestamp: 1_800_000_000,
		Signature: "signed",
		Folder:    req.GetFolder(),
		PublicId:  "public-id",
		UploadUrl: "https://api.cloudinary.com/v1_1/demo-cloud/auto/upload",
		ExpiresAt: 1_800_000_600,
	}, nil
}

type fakeFeedClient struct {
	socialflowpb.FeedServiceClient

	lastRequest *socialflowpb.GetFeedRequest
	response    *socialflowpb.GetFeedResponse
}

func (c *fakeFeedClient) GetFeed(ctx context.Context, req *socialflowpb.GetFeedRequest, _ ...grpc.CallOption) (*socialflowpb.GetFeedResponse, error) {
	c.lastRequest = req
	return c.response, nil
}

type fakeNotificationClient struct {
	socialflowpb.NotificationServiceClient

	lastListRequest *socialflowpb.GetNotificationsRequest
	lastMarkRead    *socialflowpb.MarkAsReadRequest
	response        *socialflowpb.GetNotificationsResponse
	settings        *socialflowpb.NotificationSettings
	unreadCount     int32
}

func (c *fakeNotificationClient) GetNotifications(ctx context.Context, req *socialflowpb.GetNotificationsRequest, _ ...grpc.CallOption) (*socialflowpb.GetNotificationsResponse, error) {
	c.lastListRequest = req
	return c.response, nil
}

func (c *fakeNotificationClient) GetUnreadCount(ctx context.Context, req *socialflowpb.GetUnreadCountRequest, _ ...grpc.CallOption) (*socialflowpb.GetUnreadCountResponse, error) {
	return &socialflowpb.GetUnreadCountResponse{UnreadCount: c.unreadCount}, nil
}

func (c *fakeNotificationClient) GetSettings(ctx context.Context, req *socialflowpb.GetSettingsRequest, _ ...grpc.CallOption) (*socialflowpb.GetSettingsResponse, error) {
	return &socialflowpb.GetSettingsResponse{Settings: c.settings}, nil
}

func (c *fakeNotificationClient) MarkAsRead(ctx context.Context, req *socialflowpb.MarkAsReadRequest, _ ...grpc.CallOption) (*socialflowpb.MarkAsReadResponse, error) {
	c.lastMarkRead = req
	return &socialflowpb.MarkAsReadResponse{Success: true}, nil
}

func viewerContext(userID string) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{UserID: userID, Role: "user"})
}

func TestGetFeedScopesRequestToAuthenticatedUser(t *testing.T) {
	feed := &fakeFeedClient{
		response: &socialflowpb.GetFeedResponse{
			Posts: []*socialflowpb.FeedPost{
				{Id: "p1", AuthorId: "u1", Content: "hi", LikeCount: 2},
			},
			NextCursor: "cursor-2",
			HasMore:    true,
		},
	}
	resolver := &queryResolver{&Resolver{FeedClient: feed}}

	pageSize := int32(500)
	res, err := resolver.GetFeed(viewerContext("u1"), &pageSize, nil)
	if err != nil {
		t.Fatalf("GetFeed: %v", err)
	}

	if feed.lastRequest.GetUserId() != "u1" {
		t.Fatalf("feed requested for %q, want the authenticated caller", feed.lastRequest.GetUserId())
	}
	if feed.lastRequest.GetPageSize() != maxFeedPageSize {
		t.Fatalf("page size = %d, want clamping to %d", feed.lastRequest.GetPageSize(), maxFeedPageSize)
	}
	if len(res.Posts) != 1 || res.Posts[0].ID != "p1" {
		t.Fatalf("posts = %+v", res.Posts)
	}
	if res.NextCursor == nil || *res.NextCursor != "cursor-2" {
		t.Fatalf("next cursor = %v", res.NextCursor)
	}
	if !res.HasMore {
		t.Fatal("hasMore should be true")
	}
}

func TestGetFeedRequiresAuthentication(t *testing.T) {
	resolver := &queryResolver{&Resolver{FeedClient: &fakeFeedClient{}}}

	if _, err := resolver.GetFeed(context.Background(), nil, nil); err == nil {
		t.Fatal("expected anonymous feed access to fail")
	}
}

func TestGetNotificationsPassesFilterAndUnreadCount(t *testing.T) {
	filter := model.NotificationTypePostLike
	notifications := &fakeNotificationClient{
		response: &socialflowpb.GetNotificationsResponse{
			Notifications: []*socialflowpb.NotificationItem{
				{
					Id:               "n1",
					Type:             socialflowpb.NotificationType_NOTIFICATION_TYPE_POST_LIKE,
					EntityId:         "p1",
					EntityType:       "post",
					Actors:           []*socialflowpb.NotificationActor{{Id: "u2", Username: "two"}},
					TotalActorsCount: 1,
					Title:            "New like",
					IsRead:           false,
					CreatedAt:        "2026-01-01T00:00:00Z",
				},
			},
			UnreadCount: 4,
		},
	}
	resolver := &queryResolver{&Resolver{NotificationClient: notifications}}

	res, err := resolver.GetNotifications(viewerContext("u1"), nil, nil, &filter)
	if err != nil {
		t.Fatalf("GetNotifications: %v", err)
	}

	if notifications.lastListRequest.GetUserId() != "u1" {
		t.Fatalf("notifications requested for %q", notifications.lastListRequest.GetUserId())
	}
	if notifications.lastListRequest.GetFilterType() != socialflowpb.NotificationType_NOTIFICATION_TYPE_POST_LIKE {
		t.Fatalf("filter = %v", notifications.lastListRequest.GetFilterType())
	}
	if res.UnreadCount != 4 {
		t.Fatalf("unread count = %d", res.UnreadCount)
	}
	if len(res.Notifications) != 1 || res.Notifications[0].Type != model.NotificationTypePostLike {
		t.Fatalf("notifications = %+v", res.Notifications)
	}
	if res.Notifications[0].Actors[0].Username != "two" {
		t.Fatalf("actors = %+v", res.Notifications[0].Actors)
	}
}

func TestDeletePostRejectsNonOwner(t *testing.T) {
	posts := &fakePostClient{
		posts: map[string]*socialflowpb.Post{
			"p1": {Id: "p1", AuthorId: "someone-else"},
		},
	}
	resolver := &queryResolver{&Resolver{PostClient: posts}}

	_, err := resolver.DeletePost(viewerContext("u1"), "p1")
	if err == nil {
		t.Fatal("expected deleting another user's post to fail")
	}
	gqlErr, ok := err.(*gqlerror.Error)
	if !ok || gqlErr.Extensions["code"] != "FORBIDDEN" {
		t.Fatalf("error = %#v", err)
	}
	if len(posts.deleted) != 0 {
		t.Fatalf("DeletePost should not be called, got %v", posts.deleted)
	}
}

func TestDeletePostRemovesOwnedPost(t *testing.T) {
	posts := &fakePostClient{
		posts: map[string]*socialflowpb.Post{
			"p1": {Id: "p1", AuthorId: "u1"},
		},
	}
	resolver := &queryResolver{&Resolver{PostClient: posts}}

	res, err := resolver.DeletePost(viewerContext("u1"), "p1")
	if err != nil {
		t.Fatalf("DeletePost: %v", err)
	}
	if res.Message == "" {
		t.Fatal("expected a confirmation message")
	}
	if len(posts.deleted) != 1 || posts.deleted[0] != "p1" {
		t.Fatalf("deleted = %v", posts.deleted)
	}
}

func TestMoveToArchiveRejectsNonOwner(t *testing.T) {
	posts := &fakePostClient{
		posts: map[string]*socialflowpb.Post{
			"p1": {Id: "p1", AuthorId: "someone-else"},
		},
	}
	resolver := &queryResolver{&Resolver{PostClient: posts}}

	if _, err := resolver.MoveToArchive(viewerContext("u1"), "p1"); err == nil {
		t.Fatal("expected archiving another user's post to fail")
	}
	if len(posts.archived) != 0 {
		t.Fatalf("MovePostToArchived should not be called, got %v", posts.archived)
	}
}

func TestGetUnreadNotificationCount(t *testing.T) {
	notifications := &fakeNotificationClient{unreadCount: 7}
	resolver := &queryResolver{&Resolver{NotificationClient: notifications}}

	count, err := resolver.GetUnreadNotificationCount(viewerContext("u1"))
	if err != nil {
		t.Fatalf("GetUnreadNotificationCount: %v", err)
	}
	if count != 7 {
		t.Fatalf("count = %d", count)
	}
}

func TestMarkNotificationAsReadRejectsEmptySelection(t *testing.T) {
	resolver := &mutationResolver{&Resolver{NotificationClient: &fakeNotificationClient{}}}

	if _, err := resolver.MarkNotificationAsRead(viewerContext("u1"), nil); err == nil {
		t.Fatal("expected an empty notification selection to fail")
	}
}

func TestGetUploadSignatureScopesToCaller(t *testing.T) {
	posts := &fakePostClient{}
	resolver := &queryResolver{&Resolver{PostClient: posts}}

	resourceType := "image"
	folder := "socialflow/custom"
	res, err := resolver.GetUploadSignature(viewerContext("u1"), &resourceType, &folder)
	if err != nil {
		t.Fatalf("GetUploadSignature: %v", err)
	}

	if posts.signatureRequest.GetUserId() != "u1" {
		t.Fatalf("signature requested for %q, want the authenticated caller", posts.signatureRequest.GetUserId())
	}
	if posts.signatureRequest.GetResourceType() != "image" || posts.signatureRequest.GetFolder() != "socialflow/custom" {
		t.Fatalf("request = %+v", posts.signatureRequest)
	}
	if res.CloudName != "demo-cloud" || res.Signature != "signed" {
		t.Fatalf("signature = %+v", res)
	}
	if res.Timestamp != "1800000000" || res.ExpiresAt != "1800000600" {
		t.Fatalf("timestamps = %q/%q", res.Timestamp, res.ExpiresAt)
	}
}

func TestUpdateUserRejectsAnotherUsersProfile(t *testing.T) {
	users := &fakeUserClient{}
	resolver := &mutationResolver{&Resolver{UserClient: users}}

	name := "Hijacked"
	_, err := resolver.UpdateUser(viewerContext("u1"), model.UpdateUserInput{
		ID:   "u2",
		Name: &name,
	})
	if err == nil {
		t.Fatal("expected updating another user's profile to fail")
	}
	gqlErr, ok := err.(*gqlerror.Error)
	if !ok || gqlErr.Extensions["code"] != "FORBIDDEN" {
		t.Fatalf("error = %#v", err)
	}
	if len(users.updateRequests) != 0 {
		t.Fatalf("user-service was called: %+v", users.updateRequests)
	}
}

func TestUpdateUserScopesRequestToPrincipal(t *testing.T) {
	users := &fakeUserClient{}
	resolver := &mutationResolver{&Resolver{UserClient: users}}

	name := "New Name"
	if _, err := resolver.UpdateUser(viewerContext("u1"), model.UpdateUserInput{
		ID:   "u1",
		Name: &name,
	}); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if len(users.updateRequests) != 1 || users.updateRequests[0].GetId() != "u1" {
		t.Fatalf("requests = %+v", users.updateRequests)
	}
}

func TestLoadLikeStateIsAnonymousFriendly(t *testing.T) {
	liked, err := loadLikeState(context.Background(), "p1")
	if err != nil {
		t.Fatalf("anonymous like state should not error: %v", err)
	}
	if liked {
		t.Fatal("anonymous viewers never have a like state")
	}
}

func TestNotificationItemFromFrame(t *testing.T) {
	frame := []byte(`{
		"event": "notification_received",
		"data": {
			"id": "n1",
			"type": "follow",
			"title": "New follower",
			"content_preview": "started following you",
			"action_url": "/users/u2",
			"actors": [{"id": "u2", "username": "two", "display_name": "Two"}],
			"total_actors_count": 1,
			"unread_count": 3,
			"created_at": "2026-01-01T00:00:00Z"
		}
	}`)

	item, err := notificationItemFromFrame(frame)
	if err != nil {
		t.Fatalf("notificationItemFromFrame: %v", err)
	}
	if item.ID != "n1" || item.Type != model.NotificationTypeFollow {
		t.Fatalf("item = %+v", item)
	}
	if item.EntityID != "u2" || item.EntityType != "user" {
		t.Fatalf("entity = %q/%q", item.EntityID, item.EntityType)
	}
	if len(item.Actors) != 1 || item.Actors[0].DisplayName != "Two" {
		t.Fatalf("actors = %+v", item.Actors)
	}
	if item.IsRead {
		t.Fatal("a freshly pushed notification is unread")
	}
	if item.ContentPreview == nil || *item.ContentPreview != "started following you" {
		t.Fatalf("content preview = %v", item.ContentPreview)
	}
}

func TestNotificationItemFromFrameRejectsGarbage(t *testing.T) {
	if _, err := notificationItemFromFrame([]byte("not-json")); err == nil {
		t.Fatal("expected malformed frame to fail")
	}
	if _, err := notificationItemFromFrame([]byte(`{"event":"x","data":{}}`)); err == nil {
		t.Fatal("expected frame without an id to fail")
	}
}

func TestApplySettingsOnlyOverwritesSuppliedFields(t *testing.T) {
	current := &model.NotificationSettings{
		InappLikes:    true,
		PushEnabled:   true,
		PushFollows:   true,
		EmailEnabled:  true,
		Timezone:      "UTC",
		QuietHoursEnd: "07:00",
	}
	pushLikes := false
	timezone := "Asia/Kolkata"

	next := applySettings(current, model.UpdateNotificationSettingsInput{
		PushLikes: &pushLikes,
		Timezone:  &timezone,
	})

	if next.PushLikes {
		t.Fatal("pushLikes should be updated to false")
	}
	if next.Timezone != "Asia/Kolkata" {
		t.Fatalf("timezone = %q", next.Timezone)
	}
	if !next.InappLikes || !next.PushEnabled || !next.PushFollows || !next.EmailEnabled {
		t.Fatalf("unspecified preferences were overwritten: %+v", next)
	}
	if next.QuietHoursEnd != "07:00" {
		t.Fatalf("quiet hours end = %q", next.QuietHoursEnd)
	}
}

func TestClampPageSize(t *testing.T) {
	cases := []struct {
		name    string
		request *int32
		want    int32
	}{
		{name: "nil uses fallback", request: nil, want: 20},
		{name: "zero uses fallback", request: ptrInt32(0), want: 20},
		{name: "negative uses fallback", request: ptrInt32(-3), want: 20},
		{name: "in range", request: ptrInt32(30), want: 30},
		{name: "too large is clamped", request: ptrInt32(1000), want: 50},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clampPageSize(tc.request, 20, 50); got != tc.want {
				t.Fatalf("clampPageSize = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRequireSelf(t *testing.T) {
	principal := auth.Principal{UserID: "u1"}
	if err := requireSelf(principal, "u1"); err != nil {
		t.Fatalf("own id should be allowed: %v", err)
	}
	if err := requireSelf(principal, ""); err != nil {
		t.Fatalf("empty id should defer to the caller: %v", err)
	}
	err := requireSelf(principal, "u2")
	if err == nil {
		t.Fatal("expected another user's id to be rejected")
	}
	if !strings.Contains(err.Error(), "another user") {
		t.Fatalf("error = %v", err)
	}
}

func ptrInt32(value int32) *int32 {
	return &value
}
