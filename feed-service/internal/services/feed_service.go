package services

import (
	"context"
	"encoding/base64"
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

type FeedService interface {
	GetFeed(ctx context.Context, query models.GetFeedQuery) (*models.FeedResult, error)
	InvalidateUserFeed(ctx context.Context, userID string) error
}

type feedService struct {
	feedRepo repository.FeedRepository
}

func NewFeedService(feedRepo repository.FeedRepository) FeedService {
	return &feedService{feedRepo: feedRepo}
}

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

	// 1. Fetch followed users from follow-service
	followingIDs, err := s.feedRepo.GetFollowingUserIDs(ctx, query.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve followed users: %w", err)
	}

	// 2. Build set of authors: followed accounts + user's own posts
	authorSet := make(map[string]struct{}, len(followingIDs)+1)
	for _, id := range followingIDs {
		authorSet[id] = struct{}{}
	}
	authorSet[query.UserID] = struct{}{}

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

	// 3. Fetch all posts from post-service for these authors
	rawPosts, err := s.feedRepo.GetPostsByAuthorIDs(ctx, authorIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch posts: %w", err)
	}

	// 4. Deduplicate and filter (only public posts, or user's own posts)
	postMap := make(map[string]models.FeedPost, len(rawPosts))
	for _, post := range rawPosts {
		if !post.IsPublic && post.AuthorID != query.UserID {
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

	// 6. Apply cursor filtering if provided
	cursorTime, cursorID, hasCursor := parseCursor(query.Cursor)
	if hasCursor {
		filtered := make([]models.FeedPost, 0, len(posts))
		for _, post := range posts {
			postNano := post.CreatedAt.UnixNano()
			if postNano < cursorTime || (postNano == cursorTime && post.ID < cursorID) {
				filtered = append(filtered, post)
			}
		}
		posts = filtered
	}

	// 7. Paginate results
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

func (s *feedService) InvalidateUserFeed(ctx context.Context, userID string) error {
	if _, err := uuid.Parse(userID); err != nil {
		return fmt.Errorf("%w: invalid user ID format: %v", models.ErrInvalidInput, err)
	}

	// In Phase 1 (MVP pull-model), feeds are computed dynamically on read,
	// so no cached timelines need invalidation.
	return nil
}

// encodeCursor creates an opaque base64 cursor token formatted as: <timestamp_unix_nano>:<post_id>
func encodeCursor(t time.Time, postID string) string {
	raw := fmt.Sprintf("%d:%s", t.UnixNano(), postID)
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

// parseCursor decodes the base64 cursor token into unix nanoseconds and postID.
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
