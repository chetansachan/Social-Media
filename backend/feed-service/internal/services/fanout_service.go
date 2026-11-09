package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	"github.com/chetansachan/socialflow/feed-service/internal/repository"
	"github.com/google/uuid"
)

const followBackfillLimit = 50

// FanoutService is the write-path event processor. Its methods are
// broker-agnostic: the Redis Streams consumer (or Kafka/RabbitMQ adapter)
// simply decodes an event and calls HandleEvent.
type FanoutService interface {
	FanoutPost(ctx context.Context, event models.PostCreatedEvent) error
	HandlePostDeleted(ctx context.Context, event models.PostDeletedEvent) error
	HandleUserFollowed(ctx context.Context, event models.UserFollowedEvent) error
	HandleUserUnfollowed(ctx context.Context, event models.UserUnfollowedEvent) error
	HandleEvent(ctx context.Context, event models.Event) error
}

type fanoutService struct {
	repo repository.FeedRepository
	opts Options
}

// NewFanoutService creates the hybrid fanout processor.
func NewFanoutService(repo repository.FeedRepository, opts Options) FanoutService {
	return &fanoutService{repo: repo, opts: opts.withDefaults()}
}

// FanoutPost applies the hybrid model:
//   - The author always receives their own post immediately.
//   - Private posts are not distributed.
//   - Authors at or above the celebrity threshold go into a shared pull cache.
//   - Standard authors are pushed to active followers in chunked pipelines.
func (s *fanoutService) FanoutPost(ctx context.Context, event models.PostCreatedEvent) error {
	if _, err := uuid.Parse(event.PostID); err != nil {
		return fmt.Errorf("%w: invalid post ID format: %v", models.ErrInvalidInput, err)
	}
	if _, err := uuid.Parse(event.AuthorID); err != nil {
		return fmt.Errorf("%w: invalid author ID format: %v", models.ErrInvalidInput, err)
	}

	score := models.ScoreFromTimestamp(event.CreatedAt)
	cache := s.repo.TimelineCache()

	// An author must always see their own post, even while the fanout to
	// followers is still running.
	if err := cache.PushToTimeline(ctx, event.AuthorID, event.PostID, score, s.opts.TimelineMaxSize, s.opts.TimelineTTL); err != nil {
		return fmt.Errorf("failed to push post to author timeline: %w", err)
	}

	if !event.IsPublic {
		return nil
	}

	followersCount, _, err := s.repo.GetFollowCounts(ctx, event.AuthorID)
	if err != nil {
		return fmt.Errorf("failed to classify author %s: %w", event.AuthorID, err)
	}

	if followersCount >= s.opts.CelebrityThreshold {
		// Fanout-on-read: one shared cache entry instead of millions of inbox
		// writes.
		if err := cache.StoreCelebrityPost(ctx, event.AuthorID, event.PostID, score, s.opts.CelebrityRecentLimit, s.opts.TimelineTTL); err != nil {
			return fmt.Errorf("failed to store celebrity post: %w", err)
		}
		return nil
	}

	followerIDs, err := s.repo.GetFollowerUserIDs(ctx, event.AuthorID)
	if err != nil {
		return fmt.Errorf("failed to fetch followers for fanout: %w", err)
	}
	if len(followerIDs) == 0 {
		return nil
	}

	// Do not spend Redis memory on dormant accounts. Their timeline is rebuilt
	// on demand the next time they open the app.
	active, err := cache.IsUserActiveBatch(ctx, followerIDs)
	if err != nil {
		return fmt.Errorf("failed to filter active followers: %w", err)
	}

	recipients := make([]string, 0, len(followerIDs))
	for _, followerID := range followerIDs {
		if active[followerID] {
			recipients = append(recipients, followerID)
		}
	}
	if len(recipients) == 0 {
		return nil
	}

	if err := cache.PushBatch(ctx, recipients, event.PostID, score, s.opts.TimelineMaxSize, s.opts.TimelineTTL); err != nil {
		return fmt.Errorf("failed to fan out post: %w", err)
	}
	return nil
}

// HandlePostDeleted eagerly removes the post from the author timeline and the
// celebrity cache. Follower timelines are lazily pruned during hydration, so
// this event never needs to enumerate millions of follower keys.
func (s *fanoutService) HandlePostDeleted(ctx context.Context, event models.PostDeletedEvent) error {
	if _, err := uuid.Parse(event.PostID); err != nil {
		return fmt.Errorf("%w: invalid post ID format: %v", models.ErrInvalidInput, err)
	}
	if _, err := uuid.Parse(event.AuthorID); err != nil {
		return fmt.Errorf("%w: invalid author ID format: %v", models.ErrInvalidInput, err)
	}

	cache := s.repo.TimelineCache()
	return errors.Join(
		cache.RemovePost(ctx, event.AuthorID, event.PostID),
		cache.RemoveCelebrityPost(ctx, event.AuthorID, event.PostID),
	)
}

// HandleUserFollowed backfills the followed author's recent public posts into
// the follower's timeline so the relationship is visible immediately.
func (s *fanoutService) HandleUserFollowed(ctx context.Context, event models.UserFollowedEvent) error {
	if _, err := uuid.Parse(event.FollowerID); err != nil {
		return fmt.Errorf("%w: invalid follower ID format: %v", models.ErrInvalidInput, err)
	}
	if _, err := uuid.Parse(event.FollowingID); err != nil {
		return fmt.Errorf("%w: invalid following ID format: %v", models.ErrInvalidInput, err)
	}

	_ = s.repo.TimelineCache().MarkUserActive(ctx, event.FollowerID, s.opts.ActiveUserTTL)

	posts, err := s.repo.GetPostsByAuthor(ctx, event.FollowingID)
	if err != nil {
		return fmt.Errorf("failed to fetch posts for follow backfill: %w", err)
	}

	public := make([]models.FeedPost, 0, len(posts))
	for _, post := range posts {
		if post.IsPublic {
			public = append(public, post)
		}
	}
	sortPostsDesc(public)
	if len(public) > followBackfillLimit {
		public = public[:followBackfillLimit]
	}
	if len(public) == 0 {
		return nil
	}

	items := make([]models.TimelineItem, 0, len(public))
	for _, post := range public {
		items = append(items, models.TimelineItem{
			PostID: post.ID,
			Score:  models.ScoreFromTime(post.CreatedAt),
		})
	}

	if err := s.repo.TimelineCache().BackfillTimeline(ctx, event.FollowerID, items, s.opts.TimelineMaxSize, s.opts.TimelineTTL); err != nil {
		return fmt.Errorf("failed to backfill follow timeline: %w", err)
	}
	return nil
}

// HandleUserUnfollowed purges the unfollowed author's known posts from the
// follower's cache. Any remaining references are removed lazily at read time.
func (s *fanoutService) HandleUserUnfollowed(ctx context.Context, event models.UserUnfollowedEvent) error {
	if _, err := uuid.Parse(event.FollowerID); err != nil {
		return fmt.Errorf("%w: invalid follower ID format: %v", models.ErrInvalidInput, err)
	}
	if _, err := uuid.Parse(event.FollowingID); err != nil {
		return fmt.Errorf("%w: invalid following ID format: %v", models.ErrInvalidInput, err)
	}

	posts, err := s.repo.GetPostsByAuthor(ctx, event.FollowingID)
	if err != nil {
		return fmt.Errorf("failed to fetch posts for unfollow cleanup: %w", err)
	}

	postIDs := make([]string, 0, len(posts))
	for _, post := range posts {
		postIDs = append(postIDs, post.ID)
	}

	if err := s.repo.TimelineCache().RemovePostsBatch(ctx, event.FollowerID, postIDs); err != nil {
		return fmt.Errorf("failed to purge unfollowed posts: %w", err)
	}
	return nil
}

// HandleEvent decodes the broker envelope and dispatches to the correct
// processor. Unknown event types are rejected so poison messages are visible.
func (s *fanoutService) HandleEvent(ctx context.Context, event models.Event) error {
	switch event.Type {
	case models.EventPostCreated:
		var payload models.PostCreatedEvent
		if err := event.DecodePayload(&payload); err != nil {
			return err
		}
		return s.FanoutPost(ctx, payload)

	case models.EventPostDeleted:
		var payload models.PostDeletedEvent
		if err := event.DecodePayload(&payload); err != nil {
			return err
		}
		return s.HandlePostDeleted(ctx, payload)

	case models.EventUserFollowed:
		var payload models.UserFollowedEvent
		if err := event.DecodePayload(&payload); err != nil {
			return err
		}
		return s.HandleUserFollowed(ctx, payload)

	case models.EventUserUnfollowed:
		var payload models.UserUnfollowedEvent
		if err := event.DecodePayload(&payload); err != nil {
			return err
		}
		return s.HandleUserUnfollowed(ctx, payload)

	default:
		return fmt.Errorf("%w: unknown event type %q", models.ErrInvalidInput, event.Type)
	}
}
