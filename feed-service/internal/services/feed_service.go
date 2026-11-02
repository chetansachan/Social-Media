package services

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	"github.com/chetansachan/socialflow/feed-service/internal/repository"
	"github.com/google/uuid"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// FeedService defines the core interface for newsfeed retrieval and cache management.
type FeedService interface {
	GetFeed(ctx context.Context, query models.GetFeedQuery) (*models.FeedResult, error)
	InvalidateUserFeed(ctx context.Context, userID string) error
	Fanout() FanoutService
}

type feedService struct {
	feedRepo           repository.FeedRepository
	fanoutService      FanoutService
	celebrityThreshold int32
	timelineMaxSize    int64
	timelineTTL        time.Duration
}

// NewFeedService creates an instance of the hybrid Feed Service.
func NewFeedService(
	feedRepo repository.FeedRepository,
	fanoutService FanoutService,
	celebrityThreshold int32,
	timelineMaxSize int64,
	timelineTTLHours int,
) FeedService {
	if celebrityThreshold <= 0 {
		celebrityThreshold = 25000
	}
	if timelineMaxSize <= 0 {
		timelineMaxSize = 800
	}
	ttl := time.Duration(timelineTTLHours) * time.Hour
	if ttl <= 0 {
		ttl = 720 * time.Hour
	}

	return &feedService{
		feedRepo:           feedRepo,
		fanoutService:      fanoutService,
		celebrityThreshold: celebrityThreshold,
		timelineMaxSize:    timelineMaxSize,
		timelineTTL:        ttl,
	}
}

func (s *feedService) Fanout() FanoutService {
	return s.fanoutService
}

// GetFeed executes the read path workflow:
// 1. Reads precomputed post IDs from user's Redis sorted set.
// 2. Pulls recent posts from followed celebrities and merges them in memory.
// 3. Fallback to cold-start pull model if cache is empty or Redis is unavailable.
// 4. Batch hydrates posts from Post Service concurrently.
// 5. Applies ghost post pruning for deleted or private posts.
// 6. Returns cursor-paginated FeedResult.
func (s *feedService) GetFeed(ctx context.Context, query models.GetFeedQuery) (*models.FeedResult, error) {
	if _, err := uuid.Parse(query.UserID); err != nil {
		return nil, fmt.Errorf("%w: invalid user ID format: %v", models.ErrInvalidInput, err)
	}

	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	} else if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	cursorNano, cursorID, hasCursor := parseCursor(query.Cursor)
	if query.Cursor != "" && !hasCursor {
		return nil, fmt.Errorf("%w: malformed cursor token", models.ErrInvalidCursor)
	}

	// Record user activity to keep their cache active
	_ = s.feedRepo.TimelineCache().MarkUserActive(ctx, query.UserID, s.timelineTTL)

	var cursorScore float64
	if hasCursor {
		cursorScore = float64(cursorNano)
	}

	// Step 1: Query user's Redis timeline cache
	timelineItems, err := s.feedRepo.TimelineCache().GetTimeline(
		ctx,
		query.UserID,
		cursorScore,
		int64(pageSize+1),
	)

	// Step 2: Handle Redis cache miss / unavailable / cold-start
	if err != nil || len(timelineItems) == 0 {
		return s.handleColdStartOrFallback(ctx, query.UserID, pageSize, cursorNano, cursorID, hasCursor)
	}

	// Step 3: Hybrid Pull: check if user follows celebrities and merge their recent posts
	timelineItems = s.mergeCelebrityPosts(ctx, query.UserID, timelineItems, cursorScore, cursorID, hasCursor)

	// Limit to pageSize + 1 to check if there are more pages
	if int32(len(timelineItems)) > pageSize+1 {
		timelineItems = timelineItems[:pageSize+1]
	}

	// Step 4: Batch hydrate post objects from Post Service
	postIDs := make([]string, len(timelineItems))
	for i, item := range timelineItems {
		postIDs[i] = item.PostID
	}

	hydratedPosts, err := s.feedRepo.BatchGetPosts(ctx, postIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to hydrate feed posts: %w", err)
	}

	// Step 5: Ghost Post Pruning (Problem 3)
	// Build map of hydrated posts to identify deleted or inaccessible posts
	hydratedMap := make(map[string]models.FeedPost, len(hydratedPosts))
	for _, p := range hydratedPosts {
		if !p.IsPublic && p.AuthorID != query.UserID {
			continue // private post from another author
		}
		hydratedMap[p.ID] = p
	}

	validPosts := make([]models.FeedPost, 0, len(hydratedPosts))
	for _, item := range timelineItems {
		if post, exists := hydratedMap[item.PostID]; exists {
			validPosts = append(validPosts, post)
		} else {
			// Ghost post detected: schedule background pruning from Redis
			go func(uid, pid string) {
				_ = s.feedRepo.TimelineCache().RemovePost(context.Background(), uid, pid)
			}(query.UserID, item.PostID)
		}
	}

	// If ghost pruning left us with empty posts and cache had items, try fallback
	if len(validPosts) == 0 && len(timelineItems) > 0 {
		return s.handleColdStartOrFallback(ctx, query.UserID, pageSize, cursorNano, cursorID, hasCursor)
	}

	// Step 6: Assemble paginated response
	hasMore := false
	nextCursor := ""

	if int32(len(validPosts)) > pageSize {
		hasMore = true
		validPosts = validPosts[:pageSize]
	}

	if len(validPosts) > 0 && hasMore {
		lastPost := validPosts[len(validPosts)-1]
		nextCursor = encodeCursor(lastPost.CreatedAt, lastPost.ID)
	}

	return &models.FeedResult{
		Posts:      validPosts,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// mergeCelebrityPosts pulls recent posts from followed accounts exceeding celebrity threshold
// and merges them into the precomputed timeline slice.
func (s *feedService) mergeCelebrityPosts(
	ctx context.Context,
	userID string,
	cachedItems []models.TimelineItem,
	cursorScore float64,
	cursorID string,
	hasCursor bool,
) []models.TimelineItem {
	followingIDs, err := s.feedRepo.GetFollowingUserIDs(ctx, userID)
	if err != nil || len(followingIDs) == 0 {
		return cachedItems
	}

	var celebrityItems []models.TimelineItem
	for _, aid := range followingIDs {
		followersCount, _, err := s.feedRepo.GetFollowCounts(ctx, aid)
		if err == nil && followersCount >= s.celebrityThreshold {
			// Author is a celebrity: pull recent posts
			posts, err := s.feedRepo.TimelineCache().GetCelebrityRecentPosts(ctx, aid, 20)
			if err == nil {
				celebrityItems = append(celebrityItems, posts...)
			}
		}
	}

	if len(celebrityItems) == 0 {
		return cachedItems
	}

	// Combine, deduplicate, and sort
	itemMap := make(map[string]models.TimelineItem, len(cachedItems)+len(celebrityItems))
	for _, item := range cachedItems {
		itemMap[item.PostID] = item
	}
	for _, item := range celebrityItems {
		if hasCursor {
			if item.Score > cursorScore || (item.Score == cursorScore && item.PostID >= cursorID) {
				continue // skip items after cursor
			}
		}
		itemMap[item.PostID] = item
	}

	merged := make([]models.TimelineItem, 0, len(itemMap))
	for _, item := range itemMap {
		merged = append(merged, item)
	}

	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Score == merged[j].Score {
			return merged[i].PostID > merged[j].PostID
		}
		return merged[i].Score > merged[j].Score
	})

	return merged
}

// handleColdStartOrFallback executes dynamic pull-model aggregation when the timeline cache is empty.
func (s *feedService) handleColdStartOrFallback(
	ctx context.Context,
	userID string,
	pageSize int32,
	cursorNano int64,
	cursorID string,
	hasCursor bool,
) (*models.FeedResult, error) {
	// 1. Fetch followed users from Follow Service
	followingIDs, err := s.feedRepo.GetFollowingUserIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve followed users: %w", err)
	}

	// 2. Build set of authors: followed accounts + user's own posts
	authorSet := make(map[string]struct{}, len(followingIDs)+1)
	for _, id := range followingIDs {
		authorSet[id] = struct{}{}
	}
	authorSet[userID] = struct{}{}

	authorIDs := make([]string, 0, len(authorSet))
	for id := range authorSet {
		authorIDs = append(authorIDs, id)
	}

	if len(authorIDs) == 0 {
		return &models.FeedResult{
			Posts:      []models.FeedPost{},
			NextCursor: "",
			HasMore:    false,
		}, nil
	}

	// 3. Fetch all posts from Post Service for these authors concurrently
	rawPosts, err := s.feedRepo.GetPostsByAuthorIDs(ctx, authorIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch posts: %w", err)
	}

	// 4. Deduplicate and filter (only public posts, or user's own posts)
	postMap := make(map[string]models.FeedPost, len(rawPosts))
	for _, post := range rawPosts {
		if !post.IsPublic && post.AuthorID != userID {
			continue
		}
		postMap[post.ID] = post
	}

	posts := make([]models.FeedPost, 0, len(postMap))
	for _, p := range postMap {
		posts = append(posts, p)
	}

	// 5. Sort posts in reverse chronological order (newest first)
	sort.Slice(posts, func(i, j int) bool {
		if posts[i].CreatedAt.Equal(posts[j].CreatedAt) {
			return posts[i].ID > posts[j].ID
		}
		return posts[i].CreatedAt.After(posts[j].CreatedAt)
	})

	// 6. Asynchronously warm / backfill Redis cache for future reads
	go func(uid string, postList []models.FeedPost) {
		if len(postList) == 0 {
			return
		}
		limit := s.timelineMaxSize
		if int64(len(postList)) < limit {
			limit = int64(len(postList))
		}
		items := make([]models.TimelineItem, limit)
		for i := int64(0); i < limit; i++ {
			items[i] = models.TimelineItem{
				PostID: postList[i].ID,
				Score:  float64(postList[i].CreatedAt.UnixNano()),
			}
		}
		_ = s.feedRepo.TimelineCache().BackfillTimeline(context.Background(), uid, items, s.timelineMaxSize, s.timelineTTL)
	}(userID, posts)

	// 7. Apply cursor filtering if provided
	if hasCursor {
		filtered := make([]models.FeedPost, 0, len(posts))
		for _, post := range posts {
			postNano := post.CreatedAt.UnixNano()
			if postNano < cursorNano || (postNano == cursorNano && post.ID < cursorID) {
				filtered = append(filtered, post)
			}
		}
		posts = filtered
	}

	// 8. Paginate results
	hasMore := false
	nextCursor := ""

	if int32(len(posts)) > pageSize {
		hasMore = true
		posts = posts[:pageSize]
		lastPost := posts[pageSize-1]
		nextCursor = encodeCursor(lastPost.CreatedAt, lastPost.ID)
	}

	return &models.FeedResult{
		Posts:      posts,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// InvalidateUserFeed deletes the user's timeline key in Redis, forcing a fresh pull reconstruction.
func (s *feedService) InvalidateUserFeed(ctx context.Context, userID string) error {
	if _, err := uuid.Parse(userID); err != nil {
		return fmt.Errorf("%w: invalid user ID format: %v", models.ErrInvalidInput, err)
	}

	if err := s.feedRepo.TimelineCache().InvalidateTimeline(ctx, userID); err != nil && !errors.Is(err, models.ErrCacheUnavailable) {
		return fmt.Errorf("failed to invalidate feed: %w", err)
	}
	return nil
}

// encodeCursor creates an opaque base64 cursor token formatted as: <timestamp_unix_nano>:<post_id>
func encodeCursor(t time.Time, postID string) string {
	raw := fmt.Sprintf("%d:%s", t.UnixNano(), postID)
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

// parseCursor decodes the base64 cursor token into unix nanoseconds (as int64) and postID.
func parseCursor(cursorStr string) (int64, string, bool) {
	if strings.TrimSpace(cursorStr) == "" {
		return 0, "", false
	}

	data, err := base64.URLEncoding.DecodeString(cursorStr)
	if err != nil {
		return 0, "", false
	}

	parts := strings.SplitN(string(data), ":", 2)
	if len(parts) != 2 {
		return 0, "", false
	}

	nano, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", false
	}

	return nano, parts[1], true
}
