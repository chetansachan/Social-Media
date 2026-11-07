package graph

import (
	"context"
	"testing"

	"github.com/chetansachan/socialflow/api-gateway/graph/model"
	socialflowpb "github.com/chetansachan/socialflow/proto"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"google.golang.org/grpc"
)

// The fakes below extend the ones in resolvers_test.go. Go allows a type's
// methods to live in any file of the package, so these stay next to the tests
// that need them instead of bloating the shared fixture.

func (c *fakeUserClient) Register(ctx context.Context, req *socialflowpb.RegisterUserRequest, _ ...grpc.CallOption) (*socialflowpb.RegisterUserResponse, error) {
	return &socialflowpb.RegisterUserResponse{User: &socialflowpb.User{
		Id:       "u-new",
		Username: req.GetUsername(),
		Name:     req.GetName(),
		Email:    req.GetEmail(),
		IsPublic: req.GetIsPublic(),
	}}, nil
}

func (c *fakeUserClient) Login(ctx context.Context, req *socialflowpb.LoginRequest, _ ...grpc.CallOption) (*socialflowpb.LoginResponse, error) {
	return &socialflowpb.LoginResponse{
		AccessTkn:  "access",
		RefreshTkn: "refresh",
		Message:    "ok",
	}, nil
}

func (c *fakeUserClient) Refresh(ctx context.Context, req *socialflowpb.RefreshTknRequest, _ ...grpc.CallOption) (*socialflowpb.RefreshTknResponse, error) {
	return &socialflowpb.RefreshTknResponse{
		NewAccessTkn:  "fresh-access",
		NewRefreshTkn: "fresh-refresh",
	}, nil
}

func (c *fakeUserClient) GetUserById(ctx context.Context, req *socialflowpb.GetUserByIdRequest, _ ...grpc.CallOption) (*socialflowpb.GetUserByIdResponse, error) {
	c.lastGetByID = req.GetId()
	return &socialflowpb.GetUserByIdResponse{User: &socialflowpb.User{
		Id:       req.GetId(),
		Username: "alice",
		Name:     "Alice",
	}}, nil
}

func (c *fakeUserClient) GetUserByEmail(ctx context.Context, req *socialflowpb.GetUserByEmailRequest, _ ...grpc.CallOption) (*socialflowpb.GetUserByEmailResponse, error) {
	c.lastGetByEmail = req.GetEmail()
	return &socialflowpb.GetUserByEmailResponse{User: &socialflowpb.User{
		Id:    "u1",
		Email: req.GetEmail(),
	}}, nil
}

func (c *fakeUserClient) GetUserByUsername(ctx context.Context, req *socialflowpb.GetUserByUsernameRequest, _ ...grpc.CallOption) (*socialflowpb.GetUserByUsernameResponse, error) {
	c.lastGetByUsername = req.GetUsername()
	return &socialflowpb.GetUserByUsernameResponse{User: &socialflowpb.User{
		Id:       "u1",
		Username: req.GetUsername(),
	}}, nil
}

func (c *fakeUserClient) DeleteUser(ctx context.Context, req *socialflowpb.DeleteUserRequest, _ ...grpc.CallOption) (*socialflowpb.DeleteUserResponse, error) {
	c.deletedID = req.GetId()
	return &socialflowpb.DeleteUserResponse{Message: "User Deleted Successfully"}, nil
}

func (c *fakePostClient) CreatePost(ctx context.Context, req *socialflowpb.CreatePostRequest, _ ...grpc.CallOption) (*socialflowpb.CreatePostResponse, error) {
	c.createRequests = append(c.createRequests, req)
	return &socialflowpb.CreatePostResponse{Post: &socialflowpb.Post{
		Id:       "p-new",
		AuthorId: req.GetAuthorId(),
		Content:  req.GetContent(),
	}}, nil
}

func (c *fakePostClient) UpdatePost(ctx context.Context, req *socialflowpb.UpdatePostRequest, _ ...grpc.CallOption) (*socialflowpb.UpdatePostResponse, error) {
	c.updateRequests = append(c.updateRequests, req)
	return &socialflowpb.UpdatePostResponse{Post: &socialflowpb.Post{
		Id:      req.GetPostId(),
		Content: req.GetContent(),
	}}, nil
}

func (c *fakePostClient) LikePost(ctx context.Context, req *socialflowpb.LikePostRequest, _ ...grpc.CallOption) (*socialflowpb.LikePostResponse, error) {
	c.likeRequests = append(c.likeRequests, req)
	return &socialflowpb.LikePostResponse{IsLiked: true, Message: "liked"}, nil
}

func (c *fakePostClient) GetAllPosts(ctx context.Context, req *socialflowpb.AllPostRequest, _ ...grpc.CallOption) (*socialflowpb.AllPostResponse, error) {
	c.lastAllPostsAuthor = req.GetAuthorId()
	return &socialflowpb.AllPostResponse{Post: []*socialflowpb.Post{{Id: "p1", AuthorId: req.GetAuthorId()}}}, nil
}

func (c *fakePostClient) GetAllArchivedPosts(ctx context.Context, req *socialflowpb.AllArchivedPostRequest, _ ...grpc.CallOption) (*socialflowpb.AllArchivedPostResponse, error) {
	c.lastArchivedAuthor = req.GetAuthorId()
	return &socialflowpb.AllArchivedPostResponse{Post: []*socialflowpb.Post{{Id: "a1", AuthorId: req.GetAuthorId()}}}, nil
}

func (c *fakePostClient) GetArchivedPostById(ctx context.Context, req *socialflowpb.ArchivedPostByIdRequest, _ ...grpc.CallOption) (*socialflowpb.ArchivedPostByIdResponse, error) {
	post, ok := c.archivedPosts[req.GetId()]
	if !ok {
		if c.getArchivedErrs != nil {
			return nil, c.getArchivedErrs
		}
		return nil, gqlerror.Errorf("archived post not found")
	}
	return &socialflowpb.ArchivedPostByIdResponse{Post: post}, nil
}

func (c *fakePostClient) GetAllPostLikers(ctx context.Context, req *socialflowpb.GetAllPostLikersRequest, _ ...grpc.CallOption) (*socialflowpb.GetAllPostLikersResponse, error) {
	c.lastLikersPost = req.GetPostId()
	return &socialflowpb.GetAllPostLikersResponse{UserIds: []string{"u1", "u2"}}, nil
}

func (c *fakePostClient) GetAllLikedPosts(ctx context.Context, req *socialflowpb.GetAllLikedPostsRequest, _ ...grpc.CallOption) (*socialflowpb.GetAllLikedPostsResponse, error) {
	c.lastLikedBy = req.GetUserId()
	return &socialflowpb.GetAllLikedPostsResponse{Post: []*socialflowpb.Post{{Id: "p2", AuthorId: req.GetUserId()}}}, nil
}

type fakeFollowClient struct {
	socialflowpb.FollowServiceClient

	followRequests   []*socialflowpb.FollowUserRequest
	unfollowRequests []*socialflowpb.UnfollowUserRequest
	removeRequests   []*socialflowpb.RemoveFollowerRequest
	lastStatus       *socialflowpb.GetFollowStatusRequest
	lastCounts       *socialflowpb.GetFollowCountsRequest
	lastList         *socialflowpb.GetFollowersRequest

	followers []string
	following []string
}

func (c *fakeFollowClient) FollowUser(ctx context.Context, req *socialflowpb.FollowUserRequest, _ ...grpc.CallOption) (*socialflowpb.FollowUserResponse, error) {
	c.followRequests = append(c.followRequests, req)
	return &socialflowpb.FollowUserResponse{Message: "followed"}, nil
}

func (c *fakeFollowClient) UnfollowUser(ctx context.Context, req *socialflowpb.UnfollowUserRequest, _ ...grpc.CallOption) (*socialflowpb.UnfollowUserResponse, error) {
	c.unfollowRequests = append(c.unfollowRequests, req)
	return &socialflowpb.UnfollowUserResponse{Message: "unfollowed"}, nil
}

func (c *fakeFollowClient) RemoveFollower(ctx context.Context, req *socialflowpb.RemoveFollowerRequest, _ ...grpc.CallOption) (*socialflowpb.RemoveFollowerResponse, error) {
	c.removeRequests = append(c.removeRequests, req)
	return &socialflowpb.RemoveFollowerResponse{Message: "removed"}, nil
}

func (c *fakeFollowClient) GetFollowers(ctx context.Context, req *socialflowpb.GetFollowersRequest, _ ...grpc.CallOption) (*socialflowpb.GetFollowersResponse, error) {
	c.lastList = req
	return &socialflowpb.GetFollowersResponse{FollowerIds: c.followers}, nil
}

func (c *fakeFollowClient) GetFollowing(ctx context.Context, req *socialflowpb.GetFollowingRequest, _ ...grpc.CallOption) (*socialflowpb.GetFollowingResponse, error) {
	c.lastList = &socialflowpb.GetFollowersRequest{UserId: req.GetUserId()}
	return &socialflowpb.GetFollowingResponse{FollowingIds: c.following}, nil
}

func (c *fakeFollowClient) GetFollowCounts(ctx context.Context, req *socialflowpb.GetFollowCountsRequest, _ ...grpc.CallOption) (*socialflowpb.GetFollowCountsResponse, error) {
	c.lastCounts = req
	return &socialflowpb.GetFollowCountsResponse{FollowersCount: 3, FollowingCount: 5}, nil
}

func (c *fakeFollowClient) GetFollowStatus(ctx context.Context, req *socialflowpb.GetFollowStatusRequest, _ ...grpc.CallOption) (*socialflowpb.GetFollowStatusResponse, error) {
	c.lastStatus = req
	return &socialflowpb.GetFollowStatusResponse{IsFollowing: true}, nil
}

func (c *fakeNotificationClient) MarkAllAsRead(ctx context.Context, req *socialflowpb.MarkAllAsReadRequest, _ ...grpc.CallOption) (*socialflowpb.MarkAllAsReadResponse, error) {
	c.lastMarkAll = req
	return &socialflowpb.MarkAllAsReadResponse{Success: true}, nil
}

func (c *fakeNotificationClient) DeleteNotification(ctx context.Context, req *socialflowpb.DeleteNotificationRequest, _ ...grpc.CallOption) (*socialflowpb.DeleteNotificationResponse, error) {
	c.lastDeletedNotification = req
	return &socialflowpb.DeleteNotificationResponse{Success: true}, nil
}

func (c *fakeNotificationClient) RegisterDeviceToken(ctx context.Context, req *socialflowpb.RegisterDeviceTokenRequest, _ ...grpc.CallOption) (*socialflowpb.RegisterDeviceTokenResponse, error) {
	c.lastDeviceRequest = req
	return &socialflowpb.RegisterDeviceTokenResponse{Success: true}, nil
}

func (c *fakeNotificationClient) RevokeDeviceToken(ctx context.Context, req *socialflowpb.RevokeDeviceTokenRequest, _ ...grpc.CallOption) (*socialflowpb.RevokeDeviceTokenResponse, error) {
	c.lastDeviceRequest = &socialflowpb.RegisterDeviceTokenRequest{UserId: req.GetUserId(), Token: req.GetToken()}
	return &socialflowpb.RevokeDeviceTokenResponse{Success: true}, nil
}

func (c *fakeNotificationClient) UpdateSettings(ctx context.Context, req *socialflowpb.UpdateSettingsRequest, _ ...grpc.CallOption) (*socialflowpb.UpdateSettingsResponse, error) {
	c.lastSettingsRequest = req
	return &socialflowpb.UpdateSettingsResponse{Success: true, Settings: req.GetSettings()}, nil
}

func TestRegisterAndLoginMapTokens(t *testing.T) {
	users := &fakeUserClient{}
	resolver := &mutationResolver{&Resolver{UserClient: users}}

	isPublic := false
	user, err := resolver.Register(context.Background(), model.RegisterUserInput{
		Username: "alice",
		Name:     "Alice",
		Email:    "alice@example.com",
		Password: "Passw0rd!",
		IsPublic: &isPublic,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.ID != "u-new" || user.Username != "alice" || user.IsPublic {
		t.Fatalf("user = %+v", user)
	}

	session, err := resolver.Login(context.Background(), model.LoginInput{
		Email:    "alice@example.com",
		Password: "Passw0rd!",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if session.AccessToken != "access" || session.RefressToken != "refresh" {
		t.Fatalf("session = %+v", session)
	}
}

func TestTokenGenrationMapsRefresh(t *testing.T) {
	resolver := &queryResolver{&Resolver{UserClient: &fakeUserClient{}}}

	tokens, err := resolver.TokenGenration(context.Background(), "refresh")
	if err != nil {
		t.Fatalf("TokenGenration: %v", err)
	}
	if tokens.AccessToken != "fresh-access" || tokens.RefressToken != "fresh-refresh" {
		t.Fatalf("tokens = %+v", tokens)
	}
}

func TestDeleteUserScopesToPrincipal(t *testing.T) {
	users := &fakeUserClient{}
	resolver := &queryResolver{&Resolver{UserClient: users}}

	if _, err := resolver.DeleteUser(viewerContext("u1"), "u2"); err == nil {
		t.Fatal("expected deleting another account to fail")
	}
	if users.deletedID != "" {
		t.Fatalf("DeleteUser was called for %q", users.deletedID)
	}

	res, err := resolver.DeleteUser(viewerContext("u1"), "u1")
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if users.deletedID != "u1" || res.Message == "" {
		t.Fatalf("deleted = %q, message = %q", users.deletedID, res.Message)
	}
}

func TestUserQueriesForwardTheirKey(t *testing.T) {
	users := &fakeUserClient{}
	resolver := &queryResolver{&Resolver{UserClient: users}}

	byID, err := resolver.GetUserByID(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if users.lastGetByID != "u1" || byID.Username != "alice" {
		t.Fatalf("by id: requested %q, got %+v", users.lastGetByID, byID)
	}

	byEmail, err := resolver.GetUserByEmail(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if users.lastGetByEmail != "alice@example.com" || byEmail.Email != "alice@example.com" {
		t.Fatalf("by email: %+v", byEmail)
	}

	byUsername, err := resolver.GetUserByUsername(context.Background(), "alice")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if users.lastGetByUsername != "alice" || byUsername.Username != "alice" {
		t.Fatalf("by username: %+v", byUsername)
	}
}

func TestCreatePostScopesAuthorToPrincipal(t *testing.T) {
	posts := &fakePostClient{}
	resolver := &mutationResolver{&Resolver{PostClient: posts}}

	_, err := resolver.CreatePost(viewerContext("u1"), model.CreatePostInput{
		AuthorID: "u2",
		Content:  "hello",
	})
	if err == nil {
		t.Fatal("expected a mismatched author to be rejected")
	}
	if gqlErr, ok := err.(*gqlerror.Error); !ok || gqlErr.Extensions["code"] != "FORBIDDEN" {
		t.Fatalf("error = %#v", err)
	}
	if len(posts.createRequests) != 0 {
		t.Fatal("post-service was called with a forged author")
	}

	res, err := resolver.CreatePost(viewerContext("u1"), model.CreatePostInput{
		AuthorID: "u1",
		Content:  "hello",
		MediaURL: "",
	})
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if len(posts.createRequests) != 1 || posts.createRequests[0].GetAuthorId() != "u1" {
		t.Fatalf("requests = %+v", posts.createRequests)
	}
	if res.ID != "p-new" {
		t.Fatalf("post = %+v", res)
	}
}

func TestUpdatePostRequiresOwnership(t *testing.T) {
	posts := &fakePostClient{
		posts: map[string]*socialflowpb.Post{
			"p1": {Id: "p1", AuthorId: "u1"},
			"p2": {Id: "p2", AuthorId: "someone-else"},
		},
	}
	resolver := &mutationResolver{&Resolver{PostClient: posts}}

	content := "edited"
	if _, err := resolver.UpdatePost(viewerContext("u1"), model.UpdatePostInput{
		ID:       "p1",
		AuthorID: "u1",
		Content:  &content,
	}); err != nil {
		t.Fatalf("UpdatePost: %v", err)
	}
	if len(posts.updateRequests) != 1 || posts.updateRequests[0].GetPostId() != "p1" {
		t.Fatalf("requests = %+v", posts.updateRequests)
	}

	if _, err := resolver.UpdatePost(viewerContext("u1"), model.UpdatePostInput{
		ID:       "p2",
		AuthorID: "u1",
		Content:  &content,
	}); err == nil {
		t.Fatal("expected editing another user's post to fail")
	}
	if len(posts.updateRequests) != 1 {
		t.Fatalf("service was called for a post the caller does not own: %+v", posts.updateRequests)
	}
}

func TestLikePostScopesUserToPrincipal(t *testing.T) {
	posts := &fakePostClient{}
	resolver := &mutationResolver{&Resolver{PostClient: posts}}

	res, err := resolver.LikePost(viewerContext("u1"), model.LikePostInput{PostID: "p1", UserID: "u1"})
	if err != nil {
		t.Fatalf("LikePost: %v", err)
	}
	if len(posts.likeRequests) != 1 || posts.likeRequests[0].GetUserId() != "u1" {
		t.Fatalf("requests = %+v", posts.likeRequests)
	}
	if !res.IsLiked {
		t.Fatalf("response = %+v", res)
	}
}

func TestPostListQueriesScopeCorrectly(t *testing.T) {
	posts := &fakePostClient{archivedPosts: map[string]*socialflowpb.Post{
		"a1": {Id: "a1", AuthorId: "u1"},
		"a2": {Id: "a2", AuthorId: "someone-else"},
	}}
	query := &queryResolver{&Resolver{PostClient: posts}}

	all, err := query.GetAllPost(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetAllPost: %v", err)
	}
	if posts.lastAllPostsAuthor != "u1" || len(all) != 1 {
		t.Fatalf("all = %+v (author %q)", all, posts.lastAllPostsAuthor)
	}

	// Archived posts are private to their author, so the caller must match.
	archived, err := query.GetAllArchivedPost(viewerContext("u1"), "u1")
	if err != nil {
		t.Fatalf("GetAllArchivedPost: %v", err)
	}
	if posts.lastArchivedAuthor != "u1" || len(archived) != 1 {
		t.Fatalf("archived = %+v", archived)
	}
	if _, err := query.GetAllArchivedPost(viewerContext("u1"), "u2"); err == nil {
		t.Fatal("expected another user's archive to be rejected")
	}

	likers, err := query.GetAllPostLikers(context.Background(), "p1")
	if err != nil {
		t.Fatalf("GetAllPostLikers: %v", err)
	}
	if posts.lastLikersPost != "p1" || len(likers) != 2 {
		t.Fatalf("likers = %v", likers)
	}

	liked, err := query.GetAllLikedPosts(viewerContext("u1"), "u1")
	if err != nil {
		t.Fatalf("GetAllLikedPosts: %v", err)
	}
	if posts.lastLikedBy != "u1" || len(liked) != 1 {
		t.Fatalf("liked = %+v", liked)
	}
	if _, err := query.GetAllLikedPosts(viewerContext("u1"), "u2"); err == nil {
		t.Fatal("expected another user's liked posts to be rejected")
	}

	if _, err := query.GetArchivedPostByID(viewerContext("u1"), "a2"); err == nil {
		t.Fatal("expected another user's archived post to be rejected")
	}
	mine, err := query.GetArchivedPostByID(viewerContext("u1"), "a1")
	if err != nil {
		t.Fatalf("GetArchivedPostByID: %v", err)
	}
	if mine.ID != "a1" {
		t.Fatalf("post = %+v", mine)
	}
}

func TestFollowMutationsScopeActorToPrincipal(t *testing.T) {
	follows := &fakeFollowClient{}
	resolver := &mutationResolver{&Resolver{FollowClient: follows}}

	if _, err := resolver.FollowUser(viewerContext("u1"), model.FollowUserInput{
		FollowerID:  "u2",
		FollowingID: "u3",
	}); err == nil {
		t.Fatal("expected following on someone else's behalf to fail")
	}
	if len(follows.followRequests) != 0 {
		t.Fatal("follow-service was called with a forged follower")
	}

	if _, err := resolver.FollowUser(viewerContext("u1"), model.FollowUserInput{
		FollowerID:  "u1",
		FollowingID: "u2",
	}); err != nil {
		t.Fatalf("FollowUser: %v", err)
	}
	if len(follows.followRequests) != 1 || follows.followRequests[0].GetFollowingId() != "u2" {
		t.Fatalf("follow requests = %+v", follows.followRequests)
	}

	if _, err := resolver.UnfollowUser(viewerContext("u1"), model.UnfollowUserInput{
		FollowerID:  "u1",
		FollowingID: "u2",
	}); err != nil {
		t.Fatalf("UnfollowUser: %v", err)
	}
	if len(follows.unfollowRequests) != 1 {
		t.Fatalf("unfollow requests = %+v", follows.unfollowRequests)
	}

	if _, err := resolver.RemoveFollower(viewerContext("u2"), model.RemoveFollowerInput{
		UserID:     "u2",
		FollowerID: "u1",
	}); err != nil {
		t.Fatalf("RemoveFollower: %v", err)
	}
	if len(follows.removeRequests) != 1 || follows.removeRequests[0].GetFollowerId() != "u1" {
		t.Fatalf("remove requests = %+v", follows.removeRequests)
	}
}

func TestFollowQueriesForwardTheirKey(t *testing.T) {
	follows := &fakeFollowClient{followers: []string{"u2"}, following: []string{"u3"}}
	resolver := &queryResolver{&Resolver{FollowClient: follows}}

	followers, err := resolver.GetFollowers(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetFollowers: %v", err)
	}
	if follows.lastList.GetUserId() != "u1" || len(followers) != 1 {
		t.Fatalf("followers = %v", followers)
	}

	following, err := resolver.GetFollowing(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetFollowing: %v", err)
	}
	if follows.lastList.GetUserId() != "u1" || len(following) != 1 {
		t.Fatalf("following = %v", following)
	}

	counts, err := resolver.GetFollowCounts(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetFollowCounts: %v", err)
	}
	if follows.lastCounts.GetUserId() != "u1" || counts.FollowersCount != 3 || counts.FollowingCount != 5 {
		t.Fatalf("counts = %+v", counts)
	}

	status, err := resolver.GetFollowStatus(context.Background(), "u1", "u2")
	if err != nil {
		t.Fatalf("GetFollowStatus: %v", err)
	}
	if follows.lastStatus.GetFollowerId() != "u1" || follows.lastStatus.GetFollowingId() != "u2" {
		t.Fatalf("status request = %+v", follows.lastStatus)
	}
	if !status.IsFollowing {
		t.Fatal("isFollowing should carry through")
	}
}

func TestNotificationMutationsScopeToCaller(t *testing.T) {
	notifications := &fakeNotificationClient{}
	resolver := &mutationResolver{&Resolver{NotificationClient: notifications}}

	if _, err := resolver.MarkAllNotificationsAsRead(viewerContext("u1")); err != nil {
		t.Fatalf("MarkAllNotificationsAsRead: %v", err)
	}
	if notifications.lastMarkAll.GetUserId() != "u1" {
		t.Fatalf("mark-all request = %+v", notifications.lastMarkAll)
	}

	if _, err := resolver.DeleteNotification(viewerContext("u1"), "n1"); err != nil {
		t.Fatalf("DeleteNotification: %v", err)
	}
	if notifications.lastDeletedNotification.GetUserId() != "u1" || notifications.lastDeletedNotification.GetNotificationId() != "n1" {
		t.Fatalf("delete request = %+v", notifications.lastDeletedNotification)
	}

	deviceName := "pixel"
	appVersion := "1.2.3"
	if _, err := resolver.RegisterDeviceToken(viewerContext("u1"), "token-1", "android", &deviceName, &appVersion); err != nil {
		t.Fatalf("RegisterDeviceToken: %v", err)
	}
	if notifications.lastDeviceRequest.GetUserId() != "u1" || notifications.lastDeviceRequest.GetToken() != "token-1" {
		t.Fatalf("device request = %+v", notifications.lastDeviceRequest)
	}
	if notifications.lastDeviceRequest.GetDeviceName() != deviceName || notifications.lastDeviceRequest.GetAppVersion() != appVersion {
		t.Fatalf("device metadata = %+v", notifications.lastDeviceRequest)
	}

	if _, err := resolver.RevokeDeviceToken(viewerContext("u1"), "token-1"); err != nil {
		t.Fatalf("RevokeDeviceToken: %v", err)
	}

	if _, err := resolver.MarkAllNotificationsAsRead(context.Background()); err == nil {
		t.Fatal("anonymous mark-all should fail")
	}
}

func TestUpdateNotificationSettingsReadsThenWrites(t *testing.T) {
	notifications := &fakeNotificationClient{
		settings: &socialflowpb.NotificationSettings{
			InappLikes:  true,
			PushEnabled: true,
			Timezone:    "UTC",
		},
	}
	resolver := &mutationResolver{&Resolver{NotificationClient: notifications}}

	pushLikes := false
	next, err := resolver.UpdateNotificationSettings(viewerContext("u1"), model.UpdateNotificationSettingsInput{
		PushLikes: &pushLikes,
	})
	if err != nil {
		t.Fatalf("UpdateNotificationSettings: %v", err)
	}
	if next.PushLikes {
		t.Fatal("pushLikes should now be false")
	}
	if !next.InappLikes || !next.PushEnabled {
		t.Fatalf("unspecified settings were overwritten: %+v", next)
	}
	if notifications.lastSettingsRequest.GetUserId() != "u1" {
		t.Fatalf("update request = %+v", notifications.lastSettingsRequest)
	}
	if !notifications.lastSettingsRequest.GetSettings().GetInappLikes() {
		t.Fatal("the merge should read the stored settings before writing")
	}
}

func TestGetNotificationSettingsScopesToCaller(t *testing.T) {
	resolver := &queryResolver{&Resolver{NotificationClient: &fakeNotificationClient{
		settings: &socialflowpb.NotificationSettings{InappFollows: true, Timezone: "UTC"},
	}}}

	settings, err := resolver.GetNotificationSettings(viewerContext("u1"))
	if err != nil {
		t.Fatalf("GetNotificationSettings: %v", err)
	}
	if !settings.InappFollows || settings.Timezone != "UTC" {
		t.Fatalf("settings = %+v", settings)
	}

	if _, err := resolver.GetNotificationSettings(context.Background()); err == nil {
		t.Fatal("anonymous settings read should fail")
	}
}
