package services

import (
	"context"
	"fmt"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	"github.com/chetansachan/socialflow/feed-service/internal/repository"
	"github.com/google/uuid"
)

// FanoutService handles async write-path events: post publishing fanout, deletions, and follow graph updates.
type FanoutService interface {
	FanoutPost(ctx context.Context, event models.PostCreatedEvent) error
	HandlePostDeleted(ctx context.Context, event models.PostDeletedEvent) error
	HandleUserFollowed(ctx context.Context, event models.UserFollowedEvent) error
	HandleUserUnfollowed(ctx context.Context, event models.UserUnfollowedEvent) error
}

type fanoutService struct {
	feedRepo           repository.FeedRepository
	celebrityThreshold int32
	timelineMaxSize    int64
	timelineTTL        time.Duration
}

// NewFanoutService creates a new write-path fanout processor.
func NewFanoutService(
	feedRepo repository.FeedRepository,
	celebrityThreshold int32,
	timelineMaxSize int64,
	timelineTTLHours int,
) FanoutService {
	if celebrityThreshold <= 0 {
		celebrityThreshold = 25000
	}
	if timelineMaxSize <= 0 {
		timelineMaxSize = 800
	}
	ttl := time.Duration(timelineTTLHours) * time.Hour
	if ttl <= 0 {
		ttl = 720 * time.Hour // 30 days
	}

	return &fanoutService{
		feedRepo:           feedRepo,
		celebrityThreshold: celebrityThreshold,
		timelineMaxSize:    timelineMaxSize,
		timelineTTL:        ttl,
	}
}

// FanoutPost implements the Hybrid Fanout write-path.
// Standard authors: pushes post ID to all active followers' Redis ZSET timelines.
// Celebrity authors (> 25k followers): stores in author recent feed only (pull-on-read).
func (s *fanoutService) FanoutPost(ctx context.Context, event models.PostCreatedEvent) error {
	if _, err := uuid.Parse(event.PostID); err != nil {
		return fmt.Errorf("%w: invalid post ID format: %v", models.ErrInvalidInput, err)
	}
	if _, err := uuid.Parse(event.AuthorID); err != nil {
		return fmt.Errorf("%w: invalid author ID format: %v", models.ErrInvalidInput, err)
	}

	score := float64(event.CreatedAt)
	if score <= 0 {
		score = float64(time.Now().UnixNano())
	}

	// Always push to author's own timeline so author sees their post immediately
	_ = s.feedRepo.TimelineCache().PushToTimeline(ctx, event.AuthorID, event.PostID, score, s.timelineMaxSize, s.timelineTTL)

	// If post is private, do not distribute to followers' home feeds
	if !event.IsPublic {
		return nil
	}

	// 1. Check author follower count to determine if author is a Celebrity
	followersCount, _, err := s.feedRepo.GetFollowCounts(ctx, event.AuthorID)
	if err != nil {
		// If follow count check fails, default to standard fanout
		followersCount = 0
	}

	if followersCount >= s.celebrityThreshold {
		// Model C: Celebrity accounts use Fanout-on-Read (Pull).
		// Store in author's public recent cache; skip pushing to millions of inboxes.
		return s.feedRepo.TimelineCache().StoreCelebrityPost(
			ctx,
			event.AuthorID,
			event.PostID,
			score,
			50,
			s.timelineTTL,
		)
	}

	// 2. Standard User: Push model fanout to active followers
	followerIDs, err := s.feedRepo.GetFollowerUserIDs(ctx, event.AuthorID)
	if err != nil {
		return fmt.Errorf("failed to fetch followers for fanout: %w", err)
	}

	if len(followerIDs) == 0 {
		return nil
	}

	// 3. Filter active followers to prevent wasting RAM on dormant accounts
	activeFollowers := make([]string, 0, len(followerIDs))
	for _, fid := range followerIDs {
		active, err := s.feedRepo.TimelineCache().IsUserActive(ctx, fid)
		if err == nil && active {
			activeFollowers = append(activeFollowers, fid)
		}
	}

	// If active tracking is empty or unpopulated, fallback to all followers
	recipients := activeFollowers
	if len(recipients) == 0 {
		recipients = followerIDs
	}

	// 4. Batch pipeline update into Redis Sorted Sets
	return s.feedRepo.TimelineCache().PushBatch(ctx, recipients, event.PostID, score, s.timelineMaxSize, s.timelineTTL)
}

// HandlePostDeleted eagerly invalidates post reference from author's timeline cache.
func (s *fanoutService) HandlePostDeleted(ctx context.Context, event models.PostDeletedEvent) error {
	if _, err := uuid.Parse(event.PostID); err != nil {
		return fmt.Errorf("%w: invalid post ID: %v", models.ErrInvalidInput, err)
	}
	return s.feedRepo.TimelineCache().RemovePost(ctx, event.AuthorID, event.PostID)
}

// HandleUserFollowed backfills User B's recent posts into User A's timeline cache.
func (s *fanoutService) HandleUserFollowed(ctx context.Context, event models.UserFollowedEvent) error {
	if _, err := uuid.Parse(event.FollowerID); err != nil {
		return fmt.Errorf("%w: invalid follower ID: %v", models.ErrInvalidInput, err)
	}
	if _, err := uuid.Parse(event.FollowingID); err != nil {
		return fmt.Errorf("%w: invalid following ID: %v", models.ErrInvalidInput, err)
	}

	// Fetch up to recent 50 posts from followed user
	posts, err := s.feedRepo.GetPostsByAuthor(ctx, event.FollowingID)
	if err != nil {
		return fmt.Errorf("failed fetching author posts for follow backfill: %w", err)
	}

	if len(posts) == 0 {
		return nil
	}

	items := make([]models.TimelineItem, 0, len(posts))
	for _, p := range posts {
		if !p.IsPublic {
			continue
		}
		items = append(items, models.TimelineItem{
			PostID: p.ID,
			Score:  float64(p.CreatedAt.UnixNano()),
		})
	}

	return s.feedRepo.TimelineCache().BackfillTimeline(ctx, event.FollowerID, items, s.timelineMaxSize, s.timelineTTL)
}

// HandleUserUnfollowed purges User B's posts from User A's timeline cache.
func (s *fanoutService) HandleUserUnfollowed(ctx context.Context, event models.UserUnfollowedEvent) error {
	if _, err := uuid.Parse(event.FollowerID); err != nil {
		return fmt.Errorf("%w: invalid follower ID: %v", models.ErrInvalidInput, err)
	}
	if _, err := uuid.Parse(event.FollowingID); err != nil {
		return fmt.Errorf("%w: invalid following ID: %v", models.ErrInvalidInput, err)
	}

	// Fetch followed user's posts to know which IDs to purge
	posts, err := s.feedRepo.GetPostsByAuthor(ctx, event.FollowingID)
	if err != nil {
		return nil // lazy pruning will clean these on read if fetch fails
	}

	for _, p := range posts {
		_ = s.feedRepo.TimelineCache().RemovePost(ctx, event.FollowerID, p.ID)
	}

	return nil
}
