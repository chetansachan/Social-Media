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

// FeedService is the read-path contract exposed to the gRPC transport.
type FeedService interface {
	GetFeed(ctx context.Context, query models.GetFeedQuery) (*models.FeedResult, error)
	InvalidateUserFeed(ctx context.Context, userID string) error
}

type feedService struct {
	repo repository.FeedRepository
	opts Options
}

// NewFeedService creates the hybrid read-path engine.
func NewFeedService(repo repository.FeedRepository, opts Options) FeedService {
	return &feedService{repo: repo, opts: opts.withDefaults()}
}

type feedCursor struct {
	Score  int64
	PostID string
}

// GetFeed executes the read path:
//  1. Read precomputed post IDs from the user's Redis timeline.
//  2. Merge recent posts from followed celebrity authors (hybrid pull).
//  3. Fall back to live pull aggregation on a cache miss.
//  4. Hydrate IDs from Post Service with a bounded worker pool.
//  5. Drop ghost/private posts and prune them from the cache.
//  6. Return a cursor-paginated page.
func (s *feedService) GetFeed(ctx context.Context, query models.GetFeedQuery) (*models.FeedResult, error) {
	if _, err := uuid.Parse(query.UserID); err != nil {
		return nil, fmt.Errorf("%w: invalid user ID format: %v", models.ErrInvalidInput, err)
	}

	pageSize := s.normalizePageSize(query.PageSize)

	cursor, hasCursor, ok := parseCursor(query.Cursor)
	if query.Cursor != "" && !ok {
		return nil, fmt.Errorf("%w: malformed cursor token", models.ErrInvalidCursor)
	}

	// Refresh the activity marker so future fanout writes keep this user's cache
	// warm. A failure here must not fail the read.
	_ = s.repo.TimelineCache().MarkUserActive(ctx, query.UserID, s.opts.ActiveUserTTL)

	var cursorScore int64
	if hasCursor {
		cursorScore = cursor.Score
	}

	// Fetch a small buffer beyond one page so same-timestamp ties and ghost
	// entries do not leave the page short.
	fetchLimit := int64(pageSize)*2 + 2
	timelineItems, cacheErr := s.repo.TimelineCache().GetTimeline(ctx, query.UserID, cursorScore, fetchLimit)
	if cacheErr != nil {
		// Redis is optional: degrade to live pull aggregation.
		return s.coldStart(ctx, query.UserID, pageSize, cursor, hasCursor)
	}

	if len(timelineItems) == 0 {
		// An empty cache can mean "expired/cold" or "end of a healthy
		// timeline". Cold start is safe in both cases because the cursor filter
		// guarantees that already-returned posts are not served again.
		return s.coldStart(ctx, query.UserID, pageSize, cursor, hasCursor)
	}

	candidates := filterTimelineAfterCursor(timelineItems, cursor, hasCursor)

	// Hybrid model: celebrity posts are never pushed into follower inboxes.
	// Pull the recent cache of every followed author that has one.
	celebrityItems := s.celebrityItems(ctx, query.UserID, cursor, hasCursor)
	candidates = append(candidates, celebrityItems...)
	candidates = dedupeAndSortTimeline(candidates)

	maxCandidates := int(pageSize) + 1
	if len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}

	return s.buildPage(ctx, query.UserID, candidates, pageSize)
}

// celebrityItems returns recent posts for followed authors that have a
// celebrity cache key. This avoids one GetFollowCounts call per followed user
// on every read.
func (s *feedService) celebrityItems(ctx context.Context, userID string, cursor feedCursor, hasCursor bool) []models.TimelineItem {
	followingIDs, err := s.repo.GetFollowingUserIDs(ctx, userID)
	if err != nil || len(followingIDs) == 0 {
		return nil
	}

	celebrityPosts, err := s.repo.TimelineCache().GetCelebrityRecentPostsBatch(
		ctx,
		followingIDs,
		s.opts.CelebrityRecentLimit,
	)
	if err != nil {
		return nil
	}

	var items []models.TimelineItem
	for _, authorItems := range celebrityPosts {
		items = append(items, filterTimelineAfterCursor(authorItems, cursor, hasCursor)...)
	}
	return items
}

// buildPage hydrates a candidate slice, filters deleted/private posts, prunes
// ghosts asynchronously, and assembles the final cursor-paginated page.
func (s *feedService) buildPage(ctx context.Context, userID string, candidates []models.TimelineItem, pageSize int32) (*models.FeedResult, error) {
	if len(candidates) == 0 {
		return emptyFeedResult(), nil
	}

	postIDs := make([]string, len(candidates))
	for i, item := range candidates {
		postIDs[i] = item.PostID
	}

	posts, missing, err := s.repo.HydratePosts(ctx, postIDs)
	if err != nil && len(posts) == 0 {
		return nil, fmt.Errorf("%w: %v", models.ErrFeedUnavailable, err)
	}

	byID := make(map[string]models.FeedPost, len(posts))
	for _, post := range posts {
		byID[post.ID] = post
	}

	ghosts := make([]string, 0, len(missing))
	ghosts = append(ghosts, missing...)

	valid := make([]models.FeedPost, 0, len(candidates))
	validScores := make([]int64, 0, len(candidates))
	for _, item := range candidates {
		post, exists := byID[item.PostID]
		if !exists {
			continue
		}
		if !post.IsPublic && post.AuthorID != userID {
			ghosts = append(ghosts, post.ID)
			continue
		}
		valid = append(valid, post)
		validScores = append(validScores, item.Score)
	}

	if len(ghosts) > 0 {
		s.pruneGhostsAsync(userID, dedupeStrings(ghosts))
	}

	hasMore := len(candidates) > int(pageSize) ||
		(len(ghosts) > 0 && len(valid) < int(pageSize) && len(candidates) == int(pageSize))

	if len(valid) > int(pageSize) {
		valid = valid[:pageSize]
		validScores = validScores[:pageSize]
		hasMore = true
	}

	nextCursor := ""
	if hasMore && len(valid) > 0 {
		lastIndex := len(valid) - 1
		// The cursor must use the timeline score, not the hydrated post
		// timestamp. Event timestamps and Post Service timestamps can differ by
		// a few microseconds; using the cache score guarantees the next query
		// continues strictly after the last returned entry.
		nextCursor = encodeCursorFromScore(validScores[lastIndex], valid[lastIndex].ID)
	}
	if len(valid) == 0 {
		// Every candidate was a ghost. Never advertise another page without a
		// cursor, or clients would loop on an empty response forever.
		hasMore = false
		nextCursor = ""
	}

	return &models.FeedResult{
		Posts:      valid,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// coldStart reconstructs a feed directly from Post Service when Redis has no
// precomputed timeline. The rebuilt page is asynchronously written back into
// Redis so the next scroll becomes a cache hit.
func (s *feedService) coldStart(
	ctx context.Context,
	userID string,
	pageSize int32,
	cursor feedCursor,
	hasCursor bool,
) (*models.FeedResult, error) {
	followingIDs, err := s.repo.GetFollowingUserIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to retrieve followed users: %v", models.ErrFeedUnavailable, err)
	}

	authorSet := make(map[string]struct{}, len(followingIDs)+1)
	for _, id := range followingIDs {
		authorSet[id] = struct{}{}
	}
	// A user's own posts always belong in their home feed.
	authorSet[userID] = struct{}{}

	authorIDs := make([]string, 0, len(authorSet))
	for id := range authorSet {
		authorIDs = append(authorIDs, id)
	}

	rawPosts, err := s.repo.GetPostsByAuthorIDs(ctx, authorIDs)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to fetch posts: %v", models.ErrFeedUnavailable, err)
	}

	postByID := make(map[string]models.FeedPost, len(rawPosts))
	for _, post := range rawPosts {
		if !post.IsPublic && post.AuthorID != userID {
			continue
		}
		postByID[post.ID] = post
	}

	posts := make([]models.FeedPost, 0, len(postByID))
	for _, post := range postByID {
		posts = append(posts, post)
	}
	sortPostsDesc(posts)

	// Warm the cache with the newest timelineMaxSize posts before applying the
	// pagination cursor, so the cache reflects the top of the timeline.
	s.warmCacheAsync(userID, posts)

	if hasCursor {
		posts = filterPostsAfterCursor(posts, cursor)
	}

	hasMore := len(posts) > int(pageSize)
	if hasMore {
		posts = posts[:pageSize]
	}

	nextCursor := ""
	if hasMore && len(posts) > 0 {
		last := posts[len(posts)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}

	return &models.FeedResult{
		Posts:      posts,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (s *feedService) warmCacheAsync(userID string, posts []models.FeedPost) {
	if len(posts) == 0 {
		return
	}

	limit := int(s.opts.TimelineMaxSize)
	if limit <= 0 {
		return
	}
	if len(posts) < limit {
		limit = len(posts)
	}

	items := make([]models.TimelineItem, limit)
	for i := 0; i < limit; i++ {
		items[i] = models.TimelineItem{
			PostID: posts[i].ID,
			Score:  models.ScoreFromTime(posts[i].CreatedAt),
		}
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.repo.TimelineCache().BackfillTimeline(ctx, userID, items, s.opts.TimelineMaxSize, s.opts.TimelineTTL)
	}()
}

func (s *feedService) pruneGhostsAsync(userID string, postIDs []string) {
	if len(postIDs) == 0 {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.repo.TimelineCache().RemovePostsBatch(ctx, userID, postIDs)
	}()
}

// InvalidateUserFeed deletes the precomputed timeline. The next read performs a
// cold-start rebuild from Post Service.
func (s *feedService) InvalidateUserFeed(ctx context.Context, userID string) error {
	if _, err := uuid.Parse(userID); err != nil {
		return fmt.Errorf("%w: invalid user ID format: %v", models.ErrInvalidInput, err)
	}

	if err := s.repo.TimelineCache().InvalidateTimeline(ctx, userID); err != nil {
		if errors.Is(err, models.ErrCacheUnavailable) {
			return nil
		}
		return fmt.Errorf("failed to invalidate feed: %w", err)
	}
	return nil
}

func (s *feedService) normalizePageSize(pageSize int32) int32 {
	if pageSize <= 0 {
		return s.opts.DefaultPageSize
	}
	if pageSize > s.opts.MaxPageSize {
		return s.opts.MaxPageSize
	}
	return pageSize
}

func filterTimelineAfterCursor(items []models.TimelineItem, cursor feedCursor, hasCursor bool) []models.TimelineItem {
	if !hasCursor {
		return items
	}

	filtered := make([]models.TimelineItem, 0, len(items))
	for _, item := range items {
		if item.PostID == cursor.PostID {
			continue
		}
		if item.Score < cursor.Score ||
			(item.Score == cursor.Score && item.PostID < cursor.PostID) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterPostsAfterCursor(posts []models.FeedPost, cursor feedCursor) []models.FeedPost {
	filtered := make([]models.FeedPost, 0, len(posts))
	for _, post := range posts {
		if post.ID == cursor.PostID {
			continue
		}
		score := models.ScoreFromTime(post.CreatedAt)
		if score < cursor.Score ||
			(score == cursor.Score && post.ID < cursor.PostID) {
			filtered = append(filtered, post)
		}
	}
	return filtered
}

func dedupeAndSortTimeline(items []models.TimelineItem) []models.TimelineItem {
	byID := make(map[string]models.TimelineItem, len(items))
	for _, item := range items {
		if existing, ok := byID[item.PostID]; !ok || item.Score > existing.Score {
			byID[item.PostID] = item
		}
	}

	unique := make([]models.TimelineItem, 0, len(byID))
	for _, item := range byID {
		unique = append(unique, item)
	}

	sort.Slice(unique, func(i, j int) bool {
		if unique[i].Score == unique[j].Score {
			return unique[i].PostID > unique[j].PostID
		}
		return unique[i].Score > unique[j].Score
	})
	return unique
}

func sortPostsDesc(posts []models.FeedPost) {
	sort.Slice(posts, func(i, j int) bool {
		if posts[i].CreatedAt.Equal(posts[j].CreatedAt) {
			return posts[i].ID > posts[j].ID
		}
		return posts[i].CreatedAt.After(posts[j].CreatedAt)
	})
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func emptyFeedResult() *models.FeedResult {
	return &models.FeedResult{
		Posts:      []models.FeedPost{},
		NextCursor: "",
		HasMore:    false,
	}
}

// encodeCursor creates an opaque token: base64url("<unix_micro>:<post_id>").
func encodeCursor(createdAt time.Time, postID string) string {
	return encodeCursorFromScore(createdAt.UnixMicro(), postID)
}

func encodeCursorFromScore(score int64, postID string) string {
	raw := strconv.FormatInt(score, 10) + ":" + postID
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

// parseCursor decodes a cursor token. The third return value reports whether a
// non-empty token was valid.
func parseCursor(token string) (feedCursor, bool, bool) {
	if strings.TrimSpace(token) == "" {
		return feedCursor{}, false, false
	}

	data, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		data, err = base64.RawURLEncoding.DecodeString(token)
	}
	if err != nil {
		return feedCursor{}, false, false
	}

	parts := strings.SplitN(string(data), ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return feedCursor{}, false, false
	}

	score, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || score <= 0 {
		return feedCursor{}, false, false
	}

	return feedCursor{Score: score, PostID: parts[1]}, true, true
}
