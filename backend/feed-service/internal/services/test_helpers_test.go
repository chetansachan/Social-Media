package services

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	"github.com/chetansachan/socialflow/feed-service/internal/repository"
)

type mockTimelineCache struct {
	mu          sync.Mutex
	timelines   map[string][]models.TimelineItem
	celebrity   map[string][]models.TimelineItem
	active      map[string]bool
	removed     map[string][]string
	invalidated map[string]bool
	unavailable bool
}

func newMockTimelineCache() *mockTimelineCache {
	return &mockTimelineCache{
		timelines:   make(map[string][]models.TimelineItem),
		celebrity:   make(map[string][]models.TimelineItem),
		active:      make(map[string]bool),
		removed:     make(map[string][]string),
		invalidated: make(map[string]bool),
	}
}

func (m *mockTimelineCache) GetTimeline(_ context.Context, userID string, maxScore int64, limit int64) ([]models.TimelineItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.unavailable {
		return nil, models.ErrCacheUnavailable
	}

	items := append([]models.TimelineItem(nil), m.timelines[userID]...)
	sortTimelineDesc(items)

	filtered := make([]models.TimelineItem, 0, len(items))
	for _, item := range items {
		if maxScore > 0 && item.Score > maxScore {
			continue
		}
		filtered = append(filtered, item)
		if limit > 0 && int64(len(filtered)) >= limit {
			break
		}
	}
	return filtered, nil
}

func (m *mockTimelineCache) PushToTimeline(_ context.Context, userID, postID string, score int64, maxSize int64, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.unavailable {
		return models.ErrCacheUnavailable
	}

	m.timelines[userID] = append(m.timelines[userID], models.TimelineItem{PostID: postID, Score: score})
	sortTimelineDesc(m.timelines[userID])
	m.timelines[userID] = trimTimeline(m.timelines[userID], maxSize)
	return nil
}

func (m *mockTimelineCache) PushBatch(ctx context.Context, userIDs []string, postID string, score int64, maxSize int64, ttl time.Duration) error {
	for _, userID := range userIDs {
		if err := m.PushToTimeline(ctx, userID, postID, score, maxSize, ttl); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockTimelineCache) BackfillTimeline(_ context.Context, userID string, items []models.TimelineItem, maxSize int64, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.unavailable {
		return models.ErrCacheUnavailable
	}

	existing := make(map[string]models.TimelineItem, len(m.timelines[userID])+len(items))
	for _, item := range m.timelines[userID] {
		existing[item.PostID] = item
	}
	for _, item := range items {
		existing[item.PostID] = item
	}

	merged := make([]models.TimelineItem, 0, len(existing))
	for _, item := range existing {
		merged = append(merged, item)
	}
	sortTimelineDesc(merged)
	merged = trimTimeline(merged, maxSize)
	m.timelines[userID] = merged
	return nil
}

func (m *mockTimelineCache) RemovePost(_ context.Context, userID, postID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	items := m.timelines[userID]
	filtered := make([]models.TimelineItem, 0, len(items))
	for _, item := range items {
		if item.PostID != postID {
			filtered = append(filtered, item)
		}
	}
	m.timelines[userID] = filtered
	m.removed[userID] = append(m.removed[userID], postID)
	return nil
}

func (m *mockTimelineCache) RemovePostsBatch(ctx context.Context, userID string, postIDs []string) error {
	for _, postID := range postIDs {
		if err := m.RemovePost(ctx, userID, postID); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockTimelineCache) InvalidateTimeline(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.timelines, userID)
	m.invalidated[userID] = true
	return nil
}

func (m *mockTimelineCache) StoreCelebrityPost(_ context.Context, authorID, postID string, score int64, maxItems int64, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.unavailable {
		return models.ErrCacheUnavailable
	}

	m.celebrity[authorID] = append(m.celebrity[authorID], models.TimelineItem{PostID: postID, Score: score})
	sortTimelineDesc(m.celebrity[authorID])
	m.celebrity[authorID] = trimTimeline(m.celebrity[authorID], maxItems)
	return nil
}

func (m *mockTimelineCache) GetCelebrityRecentPostsBatch(_ context.Context, authorIDs []string, limit int64) (map[string][]models.TimelineItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.unavailable {
		return nil, models.ErrCacheUnavailable
	}

	result := make(map[string][]models.TimelineItem, len(authorIDs))
	for _, authorID := range authorIDs {
		items := append([]models.TimelineItem(nil), m.celebrity[authorID]...)
		sortTimelineDesc(items)
		if limit > 0 && int64(len(items)) > limit {
			items = items[:limit]
		}
		result[authorID] = items
	}
	return result, nil
}

func (m *mockTimelineCache) RemoveCelebrityPost(_ context.Context, authorID, postID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	items := m.celebrity[authorID]
	filtered := make([]models.TimelineItem, 0, len(items))
	for _, item := range items {
		if item.PostID != postID {
			filtered = append(filtered, item)
		}
	}
	m.celebrity[authorID] = filtered
	return nil
}

func (m *mockTimelineCache) MarkUserActive(_ context.Context, userID string, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active[userID] = true
	return nil
}

func (m *mockTimelineCache) IsUserActiveBatch(_ context.Context, userIDs []string) (map[string]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.unavailable {
		return nil, models.ErrCacheUnavailable
	}

	result := make(map[string]bool, len(userIDs))
	for _, userID := range userIDs {
		result[userID] = m.active[userID]
	}
	return result, nil
}

func (m *mockTimelineCache) Ping(context.Context) error { return nil }

func (m *mockTimelineCache) setTimeline(userID string, items ...models.TimelineItem) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.timelines[userID] = append([]models.TimelineItem(nil), items...)
	sortTimelineDesc(m.timelines[userID])
}

func (m *mockTimelineCache) timeline(userID string) []models.TimelineItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]models.TimelineItem(nil), m.timelines[userID]...)
}

func (m *mockTimelineCache) removedPosts(userID string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.removed[userID]...)
}

type mockFollowClient struct {
	following map[string][]string
	followers map[string][]string
	counts    map[string][2]int32
}

func (m *mockFollowClient) GetFollowingUserIDs(_ context.Context, userID string) ([]string, error) {
	return append([]string(nil), m.following[userID]...), nil
}

func (m *mockFollowClient) GetFollowerUserIDs(_ context.Context, userID string) ([]string, error) {
	return append([]string(nil), m.followers[userID]...), nil
}

func (m *mockFollowClient) GetFollowCounts(_ context.Context, userID string) (int32, int32, error) {
	counts := m.counts[userID]
	return counts[0], counts[1], nil
}

type mockPostClient struct {
	posts     map[string]models.FeedPost
	authorErr map[string]error
}

func (m *mockPostClient) GetPostByID(_ context.Context, postID string) (*models.FeedPost, error) {
	post, ok := m.posts[postID]
	if !ok {
		return nil, nil
	}
	return &post, nil
}

func (m *mockPostClient) GetPostsByAuthor(_ context.Context, authorID string) ([]models.FeedPost, error) {
	if err := m.authorErr[authorID]; err != nil {
		return nil, err
	}
	var posts []models.FeedPost
	for _, post := range m.posts {
		if post.AuthorID == authorID {
			posts = append(posts, post)
		}
	}
	sortPostsDesc(posts)
	return posts, nil
}

func (m *mockPostClient) GetPostsByAuthorIDs(ctx context.Context, authorIDs []string) ([]models.FeedPost, error) {
	var posts []models.FeedPost
	for _, authorID := range authorIDs {
		authorPosts, err := m.GetPostsByAuthor(ctx, authorID)
		if err != nil {
			return nil, err
		}
		posts = append(posts, authorPosts...)
	}
	return posts, nil
}

func (m *mockPostClient) HydratePosts(_ context.Context, postIDs []string) ([]models.FeedPost, []string, error) {
	posts := make([]models.FeedPost, 0, len(postIDs))
	missing := make([]string, 0)
	for _, postID := range postIDs {
		post, ok := m.posts[postID]
		if !ok {
			missing = append(missing, postID)
			continue
		}
		posts = append(posts, post)
	}
	return posts, missing, nil
}

func newTestServices(tc *mockTimelineCache, fc *mockFollowClient, pc *mockPostClient) (FeedService, FanoutService, repository.FeedRepository) {
	repo := repository.NewFeedRepositoryWithClients(tc, fc, pc)
	opts := Options{
		CelebrityThreshold:   25000,
		TimelineMaxSize:      800,
		TimelineTTL:          720 * time.Hour,
		ActiveUserTTL:        14 * 24 * time.Hour,
		CelebrityRecentLimit: 20,
	}
	return NewFeedService(repo, opts), NewFanoutService(repo, opts), repo
}

func sortTimelineDesc(items []models.TimelineItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].PostID > items[j].PostID
		}
		return items[i].Score > items[j].Score
	})
}

func trimTimeline(items []models.TimelineItem, maxSize int64) []models.TimelineItem {
	if maxSize > 0 && int64(len(items)) > maxSize {
		return items[:maxSize]
	}
	return items
}

func waitFor(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}
