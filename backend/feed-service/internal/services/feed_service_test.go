package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	"github.com/google/uuid"
)

func TestGetFeed_CacheHitHydratesAndPaginatesWithoutDuplicates(t *testing.T) {
	userID := uuid.New().String()
	now := time.Now().Add(-3 * time.Hour).Truncate(time.Microsecond)

	p1, p2, p3 := uuid.New().String(), uuid.New().String(), uuid.New().String()
	tc := newMockTimelineCache()
	tc.setTimeline(userID,
		models.TimelineItem{PostID: p3, Score: models.ScoreFromTime(now.Add(2 * time.Second))},
		models.TimelineItem{PostID: p2, Score: models.ScoreFromTime(now.Add(1 * time.Second))},
		models.TimelineItem{PostID: p1, Score: models.ScoreFromTime(now)},
	)

	pc := &mockPostClient{posts: map[string]models.FeedPost{
		p1: {ID: p1, AuthorID: uuid.New().String(), Content: "one", IsPublic: true, CreatedAt: now},
		p2: {ID: p2, AuthorID: uuid.New().String(), Content: "two", IsPublic: true, CreatedAt: now.Add(time.Second)},
		p3: {ID: p3, AuthorID: uuid.New().String(), Content: "three", IsPublic: true, CreatedAt: now.Add(2 * time.Second)},
	}}
	feed, _, _ := newTestServices(tc, &mockFollowClient{}, pc)

	page1, err := feed.GetFeed(context.Background(), models.GetFeedQuery{UserID: userID, PageSize: 2})
	if err != nil {
		t.Fatalf("GetFeed page 1: %v", err)
	}
	if len(page1.Posts) != 2 || page1.Posts[0].ID != p3 || page1.Posts[1].ID != p2 {
		t.Fatalf("unexpected page 1: %+v", page1.Posts)
	}
	if !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("expected has_more=true with a cursor, got %+v", page1)
	}

	page2, err := feed.GetFeed(context.Background(), models.GetFeedQuery{
		UserID: userID, PageSize: 2, Cursor: page1.NextCursor,
	})
	if err != nil {
		t.Fatalf("GetFeed page 2: %v", err)
	}
	if len(page2.Posts) != 1 || page2.Posts[0].ID != p1 {
		t.Fatalf("expected only p1 on page 2, got %+v", page2.Posts)
	}
	if page2.HasMore {
		t.Fatalf("expected has_more=false on final page, got %+v", page2)
	}
}

func TestGetFeed_EndOfCacheDoesNotRestartAtTop(t *testing.T) {
	userID := uuid.New().String()
	postID := uuid.New().String()
	now := time.Now().Add(-time.Hour).Truncate(time.Microsecond)

	tc := newMockTimelineCache()
	tc.setTimeline(userID, models.TimelineItem{PostID: postID, Score: models.ScoreFromTime(now)})
	pc := &mockPostClient{posts: map[string]models.FeedPost{
		postID: {ID: postID, AuthorID: userID, IsPublic: true, CreatedAt: now},
	}}

	feed, _, _ := newTestServices(tc, &mockFollowClient{
		following: map[string][]string{userID: {uuid.New().String()}},
	}, pc)

	cursor := encodeCursor(now, postID)
	result, err := feed.GetFeed(context.Background(), models.GetFeedQuery{
		UserID: userID, PageSize: 20, Cursor: cursor,
	})
	if err != nil {
		t.Fatalf("GetFeed: %v", err)
	}
	if len(result.Posts) != 0 {
		t.Fatalf("expected an empty final page, got %+v", result.Posts)
	}
	if result.HasMore {
		t.Fatal("expected has_more=false at the end of the cache")
	}
}

func TestGetFeed_ColdStartFiltersPrivatePostsAndWarmsCache(t *testing.T) {
	userID := uuid.New().String()
	friendID := uuid.New().String()
	now := time.Now().Add(-4 * time.Hour).Truncate(time.Microsecond)

	ownPrivate, friendPrivate := uuid.New().String(), uuid.New().String()
	friendPublic, ownPublic := uuid.New().String(), uuid.New().String()

	tc := newMockTimelineCache()
	fc := &mockFollowClient{following: map[string][]string{userID: {friendID}}}
	pc := &mockPostClient{posts: map[string]models.FeedPost{
		ownPrivate:    {ID: ownPrivate, AuthorID: userID, IsPublic: false, CreatedAt: now.Add(3 * time.Second)},
		friendPrivate: {ID: friendPrivate, AuthorID: friendID, IsPublic: false, CreatedAt: now.Add(2 * time.Second)},
		friendPublic:  {ID: friendPublic, AuthorID: friendID, IsPublic: true, CreatedAt: now.Add(time.Second)},
		ownPublic:     {ID: ownPublic, AuthorID: userID, IsPublic: true, CreatedAt: now},
	}}

	feed, _, _ := newTestServices(tc, fc, pc)

	page1, err := feed.GetFeed(context.Background(), models.GetFeedQuery{UserID: userID, PageSize: 2})
	if err != nil {
		t.Fatalf("GetFeed page 1: %v", err)
	}
	if len(page1.Posts) != 2 || page1.Posts[0].ID != ownPrivate || page1.Posts[1].ID != friendPublic {
		t.Fatalf("unexpected cold-start page 1: %+v", page1.Posts)
	}
	if !page1.HasMore {
		t.Fatal("expected another page")
	}

	page2, err := feed.GetFeed(context.Background(), models.GetFeedQuery{
		UserID: userID, PageSize: 2, Cursor: page1.NextCursor,
	})
	if err != nil {
		t.Fatalf("GetFeed page 2: %v", err)
	}
	if len(page2.Posts) != 1 || page2.Posts[0].ID != ownPublic {
		t.Fatalf("unexpected cold-start page 2: %+v", page2.Posts)
	}
	if page2.HasMore {
		t.Fatal("expected has_more=false")
	}

	waitFor(t, "cache warm", func() bool {
		return len(tc.timeline(userID)) == 3
	})
}

func TestGetFeed_PrunesGhostAndPrivatePosts(t *testing.T) {
	userID := uuid.New().String()
	friendID := uuid.New().String()
	now := time.Now().Add(-time.Hour).Truncate(time.Microsecond)

	ghostID, privateID, validID := uuid.New().String(), uuid.New().String(), uuid.New().String()
	tc := newMockTimelineCache()
	tc.setTimeline(userID,
		models.TimelineItem{PostID: ghostID, Score: models.ScoreFromTime(now.Add(2 * time.Second))},
		models.TimelineItem{PostID: privateID, Score: models.ScoreFromTime(now.Add(time.Second))},
		models.TimelineItem{PostID: validID, Score: models.ScoreFromTime(now)},
	)

	pc := &mockPostClient{posts: map[string]models.FeedPost{
		privateID: {ID: privateID, AuthorID: friendID, IsPublic: false, CreatedAt: now.Add(time.Second)},
		validID:   {ID: validID, AuthorID: friendID, IsPublic: true, CreatedAt: now},
	}}

	feed, _, _ := newTestServices(tc, &mockFollowClient{}, pc)
	result, err := feed.GetFeed(context.Background(), models.GetFeedQuery{UserID: userID, PageSize: 10})
	if err != nil {
		t.Fatalf("GetFeed: %v", err)
	}
	if len(result.Posts) != 1 || result.Posts[0].ID != validID {
		t.Fatalf("expected only the visible post, got %+v", result.Posts)
	}

	waitFor(t, "ghost pruning", func() bool {
		removed := tc.removedPosts(userID)
		return contains(removed, ghostID) && contains(removed, privateID)
	})
}

func TestGetFeed_MergesCelebrityPosts(t *testing.T) {
	userID := uuid.New().String()
	celebrityID := uuid.New().String()
	now := time.Now().Add(-time.Hour).Truncate(time.Microsecond)

	normalID, celebrityPostID := uuid.New().String(), uuid.New().String()
	tc := newMockTimelineCache()
	tc.setTimeline(userID, models.TimelineItem{PostID: normalID, Score: models.ScoreFromTime(now)})
	_ = tc.StoreCelebrityPost(context.Background(), celebrityID, celebrityPostID, models.ScoreFromTime(now.Add(time.Second)), 20, time.Hour)

	pc := &mockPostClient{posts: map[string]models.FeedPost{
		normalID:        {ID: normalID, AuthorID: uuid.New().String(), IsPublic: true, CreatedAt: now},
		celebrityPostID: {ID: celebrityPostID, AuthorID: celebrityID, IsPublic: true, CreatedAt: now.Add(time.Second)},
	}}

	feed, _, _ := newTestServices(tc, &mockFollowClient{
		following: map[string][]string{userID: {celebrityID}},
	}, pc)

	result, err := feed.GetFeed(context.Background(), models.GetFeedQuery{UserID: userID, PageSize: 10})
	if err != nil {
		t.Fatalf("GetFeed: %v", err)
	}
	if len(result.Posts) != 2 || result.Posts[0].ID != celebrityPostID || result.Posts[1].ID != normalID {
		t.Fatalf("expected celebrity post first, got %+v", result.Posts)
	}
}

func TestGetFeed_CacheUnavailableFallsBackToPull(t *testing.T) {
	userID := uuid.New().String()
	friendID := uuid.New().String()
	postID := uuid.New().String()
	now := time.Now().Truncate(time.Microsecond)

	tc := newMockTimelineCache()
	tc.unavailable = true
	pc := &mockPostClient{posts: map[string]models.FeedPost{
		postID: {ID: postID, AuthorID: friendID, IsPublic: true, CreatedAt: now},
	}}

	feed, _, _ := newTestServices(tc, &mockFollowClient{
		following: map[string][]string{userID: {friendID}},
	}, pc)

	result, err := feed.GetFeed(context.Background(), models.GetFeedQuery{UserID: userID, PageSize: 10})
	if err != nil {
		t.Fatalf("GetFeed: %v", err)
	}
	if len(result.Posts) != 1 || result.Posts[0].ID != postID {
		t.Fatalf("expected pull fallback post, got %+v", result.Posts)
	}
}

func TestGetFeed_ValidationAndCursorRoundTrip(t *testing.T) {
	feed, _, _ := newTestServices(newMockTimelineCache(), &mockFollowClient{}, &mockPostClient{})

	if _, err := feed.GetFeed(context.Background(), models.GetFeedQuery{UserID: "not-a-uuid"}); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}

	userID := uuid.New().String()
	if _, err := feed.GetFeed(context.Background(), models.GetFeedQuery{UserID: userID, Cursor: "%%%"}); !errors.Is(err, models.ErrInvalidCursor) {
		t.Fatalf("expected ErrInvalidCursor, got %v", err)
	}

	now := time.Now().Truncate(time.Microsecond)
	postID := uuid.New().String()
	cursor, encoded, ok := parseCursor(encodeCursor(now, postID))
	if !ok || !encoded || cursor.PostID != postID || cursor.Score != now.UnixMicro() {
		t.Fatalf("cursor round-trip failed: %+v ok=%v encoded=%v", cursor, ok, encoded)
	}
}

func TestFanoutPost_StandardAuthorPushesOnlyToActiveFollowers(t *testing.T) {
	authorID, activeFollower, inactiveFollower := uuid.New().String(), uuid.New().String(), uuid.New().String()
	postID := uuid.New().String()
	now := time.Now().Truncate(time.Microsecond)

	tc := newMockTimelineCache()
	tc.active[activeFollower] = true
	fc := &mockFollowClient{
		followers: map[string][]string{authorID: {activeFollower, inactiveFollower}},
		counts:    map[string][2]int32{authorID: {2, 0}},
	}
	pc := &mockPostClient{posts: map[string]models.FeedPost{
		postID: {ID: postID, AuthorID: authorID, IsPublic: true, CreatedAt: now},
	}}

	feed, fanout, _ := newTestServices(tc, fc, pc)

	if err := fanout.FanoutPost(context.Background(), models.PostCreatedEvent{
		PostID: postID, AuthorID: authorID, CreatedAt: now.UnixMilli(), IsPublic: true,
	}); err != nil {
		t.Fatalf("FanoutPost: %v", err)
	}

	if !timelineHas(tc.timeline(authorID), postID) {
		t.Fatal("author should receive their own post")
	}
	if !timelineHas(tc.timeline(activeFollower), postID) {
		t.Fatal("active follower should receive the post")
	}
	if timelineHas(tc.timeline(inactiveFollower), postID) {
		t.Fatal("inactive follower should not receive the post")
	}

	result, err := feed.GetFeed(context.Background(), models.GetFeedQuery{UserID: activeFollower, PageSize: 10})
	if err != nil {
		t.Fatalf("GetFeed: %v", err)
	}
	if len(result.Posts) != 1 || result.Posts[0].ID != postID {
		t.Fatalf("expected pushed post in feed, got %+v", result.Posts)
	}
}

func TestFanoutPost_CelebrityUsesSharedPullCache(t *testing.T) {
	celebrityID, followerID, postID := uuid.New().String(), uuid.New().String(), uuid.New().String()
	now := time.Now().Truncate(time.Microsecond)

	tc := newMockTimelineCache()
	tc.active[followerID] = true
	fc := &mockFollowClient{
		followers: map[string][]string{celebrityID: {followerID}},
		counts:    map[string][2]int32{celebrityID: {50000, 10}},
	}

	_, fanout, _ := newTestServices(tc, fc, &mockPostClient{})
	if err := fanout.FanoutPost(context.Background(), models.PostCreatedEvent{
		PostID: postID, AuthorID: celebrityID, CreatedAt: now.UnixMilli(), IsPublic: true,
	}); err != nil {
		t.Fatalf("FanoutPost: %v", err)
	}

	if timelineHas(tc.timeline(followerID), postID) {
		t.Fatal("celebrity posts must not be pushed into follower inboxes")
	}

	tc.mu.Lock()
	celebrityItems := append([]models.TimelineItem(nil), tc.celebrity[celebrityID]...)
	tc.mu.Unlock()
	if !timelineHas(celebrityItems, postID) {
		t.Fatal("celebrity post should be stored in the shared recent cache")
	}
	if !timelineHas(tc.timeline(celebrityID), postID) {
		t.Fatal("author should still receive their own post")
	}
}

func TestFanoutPost_PrivatePostStaysWithAuthor(t *testing.T) {
	authorID, followerID, postID := uuid.New().String(), uuid.New().String(), uuid.New().String()

	tc := newMockTimelineCache()
	tc.active[followerID] = true
	fc := &mockFollowClient{
		followers: map[string][]string{authorID: {followerID}},
		counts:    map[string][2]int32{authorID: {1, 0}},
	}

	_, fanout, _ := newTestServices(tc, fc, &mockPostClient{})
	if err := fanout.FanoutPost(context.Background(), models.PostCreatedEvent{
		PostID: postID, AuthorID: authorID, CreatedAt: time.Now().UnixMilli(), IsPublic: false,
	}); err != nil {
		t.Fatalf("FanoutPost: %v", err)
	}

	if !timelineHas(tc.timeline(authorID), postID) {
		t.Fatal("author should see their own private post")
	}
	if timelineHas(tc.timeline(followerID), postID) {
		t.Fatal("private post must not reach followers")
	}
}

func TestHandlePostDeletedRemovesAuthorAndCelebrityEntries(t *testing.T) {
	authorID, postID := uuid.New().String(), uuid.New().String()
	now := time.Now().UnixMicro()

	tc := newMockTimelineCache()
	tc.setTimeline(authorID, models.TimelineItem{PostID: postID, Score: now})
	_ = tc.StoreCelebrityPost(context.Background(), authorID, postID, now, 20, time.Hour)

	_, fanout, _ := newTestServices(tc, &mockFollowClient{}, &mockPostClient{})
	if err := fanout.HandlePostDeleted(context.Background(), models.PostDeletedEvent{
		PostID: postID, AuthorID: authorID,
	}); err != nil {
		t.Fatalf("HandlePostDeleted: %v", err)
	}

	if timelineHas(tc.timeline(authorID), postID) {
		t.Fatal("deleted post should be removed from author timeline")
	}
	tc.mu.Lock()
	celebrityItems := append([]models.TimelineItem(nil), tc.celebrity[authorID]...)
	tc.mu.Unlock()
	if timelineHas(celebrityItems, postID) {
		t.Fatal("deleted post should be removed from celebrity cache")
	}
}

func TestFollowBackfillCapsPostsAndUnfollowPurgesTimeline(t *testing.T) {
	userA, userB := uuid.New().String(), uuid.New().String()
	now := time.Now().Truncate(time.Microsecond)

	pc := &mockPostClient{posts: make(map[string]models.FeedPost)}
	for i := 0; i < 60; i++ {
		postID := uuid.New().String()
		pc.posts[postID] = models.FeedPost{
			ID:        postID,
			AuthorID:  userB,
			IsPublic:  true,
			CreatedAt: now.Add(-time.Duration(i) * time.Second),
		}
	}

	tc := newMockTimelineCache()
	_, fanout, _ := newTestServices(tc, &mockFollowClient{}, pc)

	if err := fanout.HandleUserFollowed(context.Background(), models.UserFollowedEvent{
		FollowerID: userA, FollowingID: userB,
	}); err != nil {
		t.Fatalf("HandleUserFollowed: %v", err)
	}
	if got := len(tc.timeline(userA)); got != followBackfillLimit {
		t.Fatalf("expected %d backfilled posts, got %d", followBackfillLimit, got)
	}

	if err := fanout.HandleUserUnfollowed(context.Background(), models.UserUnfollowedEvent{
		FollowerID: userA, FollowingID: userB,
	}); err != nil {
		t.Fatalf("HandleUserUnfollowed: %v", err)
	}
	if got := len(tc.timeline(userA)); got != 0 {
		t.Fatalf("expected an empty timeline after unfollow, got %d items", got)
	}
}

func TestHandleEventDispatchesPostCreated(t *testing.T) {
	authorID, postID := uuid.New().String(), uuid.New().String()
	payload, err := json.Marshal(models.PostCreatedEvent{
		PostID: postID, AuthorID: authorID, CreatedAt: time.Now().UnixMilli(), IsPublic: true,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	tc := newMockTimelineCache()
	fc := &mockFollowClient{
		followers: map[string][]string{},
		counts:    map[string][2]int32{authorID: {0, 0}},
	}
	_, fanout, _ := newTestServices(tc, fc, &mockPostClient{})

	err = fanout.HandleEvent(context.Background(), models.Event{
		Type:    models.EventPostCreated,
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if !timelineHas(tc.timeline(authorID), postID) {
		t.Fatal("dispatched event should fan out the post")
	}

	err = fanout.HandleEvent(context.Background(), models.Event{Type: "unknown", Payload: payload})
	if !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for unknown event, got %v", err)
	}
}

func TestInvalidateUserFeed(t *testing.T) {
	userID := uuid.New().String()
	tc := newMockTimelineCache()
	tc.setTimeline(userID, models.TimelineItem{PostID: uuid.New().String(), Score: time.Now().UnixMicro()})

	feed, _, _ := newTestServices(tc, &mockFollowClient{}, &mockPostClient{})
	if err := feed.InvalidateUserFeed(context.Background(), userID); err != nil {
		t.Fatalf("InvalidateUserFeed: %v", err)
	}
	if !tc.invalidated[userID] {
		t.Fatal("expected timeline to be invalidated")
	}
}

func timelineHas(items []models.TimelineItem, postID string) bool {
	for _, item := range items {
		if item.PostID == postID {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
