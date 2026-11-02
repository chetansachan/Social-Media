package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	"github.com/chetansachan/socialflow/feed-service/internal/repository"
	"github.com/google/uuid"
)

type mockTimelineCache struct {
	timelines        map[string][]models.TimelineItem
	celebrityPosts   map[string][]models.TimelineItem
	activeUsers      map[string]bool
	invalidatedUsers []string
}

func newMockTimelineCache() *mockTimelineCache {
	return &mockTimelineCache{
		timelines:      make(map[string][]models.TimelineItem),
		celebrityPosts: make(map[string][]models.TimelineItem),
		activeUsers:    make(map[string]bool),
	}
}

func (m *mockTimelineCache) GetTimeline(ctx context.Context, userID string, maxScore float64, limit int64) ([]models.TimelineItem, error) {
	items := m.timelines[userID]
	if len(items) == 0 {
		return []models.TimelineItem{}, nil
	}

	filtered := make([]models.TimelineItem, 0, len(items))
	for _, item := range items {
		if maxScore > 0 && item.Score >= maxScore {
			continue
		}
		filtered = append(filtered, item)
		if limit > 0 && int64(len(filtered)) >= limit {
			break
		}
	}
	return filtered, nil
}

func (m *mockTimelineCache) PushToTimeline(ctx context.Context, userID string, postID string, score float64, maxSize int64, ttl time.Duration) error {
	item := models.TimelineItem{PostID: postID, Score: score}
	m.timelines[userID] = append([]models.TimelineItem{item}, m.timelines[userID]...)
	if maxSize > 0 && int64(len(m.timelines[userID])) > maxSize {
		m.timelines[userID] = m.timelines[userID][:maxSize]
	}
	return nil
}

func (m *mockTimelineCache) PushBatch(ctx context.Context, userIDs []string, postID string, score float64, maxSize int64, ttl time.Duration) error {
	for _, uid := range userIDs {
		_ = m.PushToTimeline(ctx, uid, postID, score, maxSize, ttl)
	}
	return nil
}

func (m *mockTimelineCache) RemovePost(ctx context.Context, userID string, postID string) error {
	items := m.timelines[userID]
	filtered := make([]models.TimelineItem, 0, len(items))
	for _, item := range items {
		if item.PostID != postID {
			filtered = append(filtered, item)
		}
	}
	m.timelines[userID] = filtered
	return nil
}

func (m *mockTimelineCache) RemovePostBatch(ctx context.Context, userIDs []string, postID string) error {
	for _, uid := range userIDs {
		_ = m.RemovePost(ctx, uid, postID)
	}
	return nil
}

func (m *mockTimelineCache) InvalidateTimeline(ctx context.Context, userID string) error {
	delete(m.timelines, userID)
	m.invalidatedUsers = append(m.invalidatedUsers, userID)
	return nil
}

func (m *mockTimelineCache) BackfillTimeline(ctx context.Context, userID string, items []models.TimelineItem, maxSize int64, ttl time.Duration) error {
	existing := m.timelines[userID]
	combined := append(existing, items...)
	m.timelines[userID] = combined
	return nil
}

func (m *mockTimelineCache) StoreCelebrityPost(ctx context.Context, authorID string, postID string, score float64, maxItems int64, ttl time.Duration) error {
	item := models.TimelineItem{PostID: postID, Score: score}
	m.celebrityPosts[authorID] = append([]models.TimelineItem{item}, m.celebrityPosts[authorID]...)
	return nil
}

func (m *mockTimelineCache) GetCelebrityRecentPosts(ctx context.Context, authorID string, limit int64) ([]models.TimelineItem, error) {
	posts := m.celebrityPosts[authorID]
	if limit > 0 && int64(len(posts)) > limit {
		posts = posts[:limit]
	}
	return posts, nil
}

func (m *mockTimelineCache) MarkUserActive(ctx context.Context, userID string, ttl time.Duration) error {
	m.activeUsers[userID] = true
	return nil
}

func (m *mockTimelineCache) IsUserActive(ctx context.Context, userID string) (bool, error) {
	active, exists := m.activeUsers[userID]
	if !exists {
		return true, nil // default active
	}
	return active, nil
}

func (m *mockTimelineCache) Ping(ctx context.Context) error {
	return nil
}

type mockFollowClient struct {
	followingMap map[string][]string
	followerMap  map[string][]string
	countsMap    map[string][2]int32
}

func (m *mockFollowClient) GetFollowingUserIDs(ctx context.Context, userID string) ([]string, error) {
	return m.followingMap[userID], nil
}

func (m *mockFollowClient) GetFollowerUserIDs(ctx context.Context, userID string) ([]string, error) {
	return m.followerMap[userID], nil
}

func (m *mockFollowClient) GetFollowCounts(ctx context.Context, userID string) (int32, int32, error) {
	counts, exists := m.countsMap[userID]
	if !exists {
		return 0, 0, nil
	}
	return counts[0], counts[1], nil
}

type mockPostClient struct {
	postsMap map[string]models.FeedPost
}

func (m *mockPostClient) GetPostByID(ctx context.Context, postID string) (*models.FeedPost, error) {
	p, exists := m.postsMap[postID]
	if !exists {
		return nil, nil
	}
	return &p, nil
}

func (m *mockPostClient) GetPostsByAuthor(ctx context.Context, authorID string) ([]models.FeedPost, error) {
	var list []models.FeedPost
	for _, p := range m.postsMap {
		if p.AuthorID == authorID {
			list = append(list, p)
		}
	}
	return list, nil
}

func (m *mockPostClient) GetPostsByAuthorIDs(ctx context.Context, authorIDs []string) ([]models.FeedPost, error) {
	authorSet := make(map[string]bool, len(authorIDs))
	for _, id := range authorIDs {
		authorSet[id] = true
	}
	var list []models.FeedPost
	for _, p := range m.postsMap {
		if authorSet[p.AuthorID] {
			list = append(list, p)
		}
	}
	return list, nil
}

func (m *mockPostClient) BatchGetPosts(ctx context.Context, postIDs []string) ([]models.FeedPost, error) {
	var list []models.FeedPost
	for _, pid := range postIDs {
		if p, exists := m.postsMap[pid]; exists {
			list = append(list, p)
		}
	}
	return list, nil
}

func setupTestService(
	tc *mockTimelineCache,
	fc *mockFollowClient,
	pc *mockPostClient,
	celebrityThreshold int32,
) (FeedService, FanoutService) {
	repo := repository.NewFeedRepositoryWithClients(tc, fc, pc)
	fanout := NewFanoutService(repo, celebrityThreshold, 800, 720)
	feed := NewFeedService(repo, fanout, celebrityThreshold, 800, 720)
	return feed, fanout
}

func TestGetFeed_InvalidUserID(t *testing.T) {
	feedSvc, _ := setupTestService(newMockTimelineCache(), &mockFollowClient{}, &mockPostClient{}, 25000)

	_, err := feedSvc.GetFeed(context.Background(), models.GetFeedQuery{
		UserID: "not-a-uuid",
	})
	if err == nil {
		t.Fatal("expected error for invalid user ID, got nil")
	}
	if !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestGetFeed_InvalidCursor(t *testing.T) {
	feedSvc, _ := setupTestService(newMockTimelineCache(), &mockFollowClient{}, &mockPostClient{}, 25000)

	_, err := feedSvc.GetFeed(context.Background(), models.GetFeedQuery{
		UserID: uuid.New().String(),
		Cursor: "%%%not-valid-base64%%%",
	})
	if err == nil {
		t.Fatal("expected error for invalid cursor, got nil")
	}
	if !errors.Is(err, models.ErrInvalidCursor) {
		t.Fatalf("expected ErrInvalidCursor, got %v", err)
	}
}

func TestGetFeed_EmptyFeed(t *testing.T) {
	userID := uuid.New().String()
	tc := newMockTimelineCache()
	fc := &mockFollowClient{followingMap: map[string][]string{userID: {}}}
	pc := &mockPostClient{postsMap: map[string]models.FeedPost{}}

	feedSvc, _ := setupTestService(tc, fc, pc, 25000)

	res, err := feedSvc.GetFeed(context.Background(), models.GetFeedQuery{
		UserID:   userID,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Posts) != 0 {
		t.Fatalf("expected 0 posts, got %d", len(res.Posts))
	}
	if res.HasMore {
		t.Fatal("expected HasMore to be false")
	}
	if res.NextCursor != "" {
		t.Fatalf("expected empty NextCursor, got %s", res.NextCursor)
	}
}

func TestGetFeed_ColdStartFallback_SortFilterAndPaginate(t *testing.T) {
	userID := uuid.New().String()
	friendID := uuid.New().String()

	now := time.Now()
	t1 := now.Add(-3 * time.Hour)
	t2 := now.Add(-2 * time.Hour)
	t3 := now.Add(-1 * time.Hour)
	t4 := now

	tc := newMockTimelineCache()
	fc := &mockFollowClient{
		followingMap: map[string][]string{userID: {friendID}},
	}
	pc := &mockPostClient{
		postsMap: map[string]models.FeedPost{
			"p1": {ID: "p1", AuthorID: friendID, Content: "Old post", IsPublic: true, CreatedAt: t1},
			"p3": {ID: "p3", AuthorID: friendID, Content: "Recent public post", IsPublic: true, CreatedAt: t3},
			"p-priv": {ID: "p-priv", AuthorID: friendID, Content: "Friend's private post", IsPublic: false, CreatedAt: t4},
			"p2": {ID: "p2", AuthorID: userID, Content: "My older post", IsPublic: true, CreatedAt: t2},
			"p4": {ID: "p4", AuthorID: userID, Content: "My newest private post", IsPublic: false, CreatedAt: t4},
		},
	}

	feedSvc, _ := setupTestService(tc, fc, pc, 25000)

	// Page 1: PageSize = 2
	res, err := feedSvc.GetFeed(context.Background(), models.GetFeedQuery{
		UserID:   userID,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Friend's private post excluded; user's own private post included.
	// Order: p4, p3, p2, p1
	if len(res.Posts) != 2 {
		t.Fatalf("expected 2 posts for page 1, got %d", len(res.Posts))
	}
	if res.Posts[0].ID != "p4" {
		t.Errorf("expected first post to be p4, got %s", res.Posts[0].ID)
	}
	if res.Posts[1].ID != "p3" {
		t.Errorf("expected second post to be p3, got %s", res.Posts[1].ID)
	}
	if !res.HasMore {
		t.Errorf("expected HasMore to be true")
	}

	// Page 2: Fetch next page using NextCursor
	res2, err := feedSvc.GetFeed(context.Background(), models.GetFeedQuery{
		UserID:   userID,
		PageSize: 2,
		Cursor:   res.NextCursor,
	})
	if err != nil {
		t.Fatalf("unexpected error fetching page 2: %v", err)
	}
	if len(res2.Posts) != 2 {
		t.Fatalf("expected 2 posts for page 2, got %d", len(res2.Posts))
	}
	if res2.Posts[0].ID != "p2" {
		t.Errorf("expected first post on page 2 to be p2, got %s", res2.Posts[0].ID)
	}
	if res2.Posts[1].ID != "p1" {
		t.Errorf("expected second post on page 2 to be p1, got %s", res2.Posts[1].ID)
	}
	if res2.HasMore {
		t.Errorf("expected HasMore to be false on last page")
	}
}

func TestFanoutAndCacheRead(t *testing.T) {
	authorID := uuid.New().String()
	follower1 := uuid.New().String()
	follower2 := uuid.New().String()
	postID := uuid.New().String()

	tc := newMockTimelineCache()
	fc := &mockFollowClient{
		followerMap: map[string][]string{
			authorID: {follower1, follower2},
		},
		countsMap: map[string][2]int32{
			authorID: {2, 0},
		},
	}
	pc := &mockPostClient{
		postsMap: map[string]models.FeedPost{
			postID: {
				ID:        postID,
				AuthorID:  authorID,
				Content:   "Pushed post to followers",
				IsPublic:  true,
				CreatedAt: time.Now(),
			},
		},
	}

	feedSvc, fanoutSvc := setupTestService(tc, fc, pc, 25000)

	// Publish post via Fanout
	err := fanoutSvc.FanoutPost(context.Background(), models.PostCreatedEvent{
		PostID:    postID,
		AuthorID:  authorID,
		CreatedAt: time.Now().UnixNano(),
		IsPublic:  true,
	})
	if err != nil {
		t.Fatalf("fanout post failed: %v", err)
	}

	// Follower 1 should now read the post directly from cache
	res, err := feedSvc.GetFeed(context.Background(), models.GetFeedQuery{
		UserID:   follower1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("failed to get feed for follower1: %v", err)
	}
	if len(res.Posts) != 1 || res.Posts[0].ID != postID {
		t.Fatalf("expected follower1 to receive post %s, got %+v", postID, res.Posts)
	}
}

func TestCelebrityHybridFanout(t *testing.T) {
	celebrityID := uuid.New().String()
	followerID := uuid.New().String()
	celebPostID := uuid.New().String()

	tc := newMockTimelineCache()
	fc := &mockFollowClient{
		followingMap: map[string][]string{
			followerID: {celebrityID},
		},
		followerMap: map[string][]string{
			celebrityID: {followerID},
		},
		countsMap: map[string][2]int32{
			celebrityID: {50000, 10}, // > 25000 celebrity threshold
		},
	}
	pc := &mockPostClient{
		postsMap: map[string]models.FeedPost{
			celebPostID: {
				ID:        celebPostID,
				AuthorID:  celebrityID,
				Content:   "Celebrity announcement!",
				IsPublic:  true,
				CreatedAt: time.Now(),
			},
		},
	}

	feedSvc, fanoutSvc := setupTestService(tc, fc, pc, 25000)

	// Fanout celebrity post: should NOT push to follower's inbox directly
	err := fanoutSvc.FanoutPost(context.Background(), models.PostCreatedEvent{
		PostID:    celebPostID,
		AuthorID:  celebrityID,
		CreatedAt: time.Now().UnixNano(),
		IsPublic:  true,
	})
	if err != nil {
		t.Fatalf("celebrity fanout failed: %v", err)
	}

	// Verify follower's push timeline is empty (no push write amplification)
	if len(tc.timelines[followerID]) != 0 {
		t.Fatalf("expected follower timeline to be empty for celebrity post, got %d items", len(tc.timelines[followerID]))
	}

	// Push a normal post to follower's timeline so cache is active
	normalPostID := uuid.New().String()
	pc.postsMap[normalPostID] = models.FeedPost{
		ID:        normalPostID,
		AuthorID:  followerID,
		Content:   "My regular post",
		IsPublic:  true,
		CreatedAt: time.Now().Add(-10 * time.Minute),
	}
	_ = tc.PushToTimeline(context.Background(), followerID, normalPostID, float64(time.Now().Add(-10*time.Minute).UnixNano()), 800, time.Hour)

	// When follower reads feed, hybrid pull fetches celebrity post and merges
	res, err := feedSvc.GetFeed(context.Background(), models.GetFeedQuery{
		UserID:   followerID,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("get feed failed: %v", err)
	}

	if len(res.Posts) != 2 {
		t.Fatalf("expected 2 merged posts (1 normal + 1 celebrity), got %d", len(res.Posts))
	}
	if res.Posts[0].ID != celebPostID {
		t.Errorf("expected first post to be celebrity post %s, got %s", celebPostID, res.Posts[0].ID)
	}
}

func TestFollowBackfillAndUnfollow(t *testing.T) {
	userA := uuid.New().String()
	userB := uuid.New().String()
	postB := uuid.New().String()

	tc := newMockTimelineCache()
	fc := &mockFollowClient{}
	pc := &mockPostClient{
		postsMap: map[string]models.FeedPost{
			postB: {
				ID:        postB,
				AuthorID:  userB,
				Content:   "User B post",
				IsPublic:  true,
				CreatedAt: time.Now(),
			},
		},
	}

	_, fanoutSvc := setupTestService(tc, fc, pc, 25000)

	// User A follows User B: backfill B's post into A's timeline
	err := fanoutSvc.HandleUserFollowed(context.Background(), models.UserFollowedEvent{
		FollowerID:  userA,
		FollowingID: userB,
	})
	if err != nil {
		t.Fatalf("follow backfill failed: %v", err)
	}

	if len(tc.timelines[userA]) != 1 || tc.timelines[userA][0].PostID != postB {
		t.Fatalf("expected post %s backfilled into userA timeline, got %+v", postB, tc.timelines[userA])
	}

	// User A unfollows User B: purge B's posts from A's timeline
	err = fanoutSvc.HandleUserUnfollowed(context.Background(), models.UserUnfollowedEvent{
		FollowerID:  userA,
		FollowingID: userB,
	})
	if err != nil {
		t.Fatalf("unfollow purge failed: %v", err)
	}

	if len(tc.timelines[userA]) != 0 {
		t.Fatalf("expected userA timeline to be empty after unfollow, got %d items", len(tc.timelines[userA]))
	}
}

func TestInvalidateUserFeed(t *testing.T) {
	userID := uuid.New().String()
	tc := newMockTimelineCache()
	_ = tc.PushToTimeline(context.Background(), userID, "post-1", 100, 800, time.Hour)

	feedSvc, _ := setupTestService(tc, &mockFollowClient{}, &mockPostClient{}, 25000)

	err := feedSvc.InvalidateUserFeed(context.Background(), userID)
	if err != nil {
		t.Fatalf("invalidate feed failed: %v", err)
	}

	if len(tc.timelines[userID]) != 0 {
		t.Fatalf("expected timeline to be deleted after invalidation")
	}
}

func TestCursorEncodingRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Nanosecond)
	postID := "test-post-uuid-1234"

	cursor := encodeCursor(now, postID)
	nano, id, ok := parseCursor(cursor)
	if !ok {
		t.Fatal("expected parseCursor to return ok=true")
	}
	if nano != now.UnixNano() {
		t.Errorf("expected nano %d, got %d", now.UnixNano(), nano)
	}
	if id != postID {
		t.Errorf("expected id %s, got %s", postID, id)
	}
}
