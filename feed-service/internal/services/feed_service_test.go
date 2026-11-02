package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	"github.com/google/uuid"
)

// mockFeedRepository implements repository.FeedRepository for testing
type mockFeedRepository struct {
	followingMap map[string][]string
	postsMap     map[string][]models.FeedPost
	err          error
}

func (m *mockFeedRepository) GetFollowingUserIDs(ctx context.Context, userID string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.followingMap[userID], nil
}

func (m *mockFeedRepository) GetPostsByAuthor(ctx context.Context, authorID string) ([]models.FeedPost, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.postsMap[authorID], nil
}

func (m *mockFeedRepository) GetPostsByAuthorIDs(ctx context.Context, authorIDs []string) ([]models.FeedPost, error) {
	if m.err != nil {
		return nil, m.err
	}
	var all []models.FeedPost
	for _, aid := range authorIDs {
		all = append(all, m.postsMap[aid]...)
	}
	return all, nil
}

func TestGetFeed_InvalidUserID(t *testing.T) {
	repo := &mockFeedRepository{}
	svc := NewFeedService(repo)

	_, err := svc.GetFeed(context.Background(), models.GetFeedQuery{
		UserID: "invalid-uuid",
	})
	if err == nil {
		t.Fatal("expected error for invalid user ID, got nil")
	}
	if !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestGetFeed_EmptyFeed(t *testing.T) {
	userID := uuid.New().String()
	repo := &mockFeedRepository{
		followingMap: map[string][]string{userID: {}},
		postsMap:     map[string][]models.FeedPost{},
	}
	svc := NewFeedService(repo)

	res, err := svc.GetFeed(context.Background(), models.GetFeedQuery{
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

func TestGetFeed_SortFilterAndPaginate(t *testing.T) {
	userID := uuid.New().String()
	friendID := uuid.New().String()

	now := time.Now()
	t1 := now.Add(-3 * time.Hour)
	t2 := now.Add(-2 * time.Hour)
	t3 := now.Add(-1 * time.Hour)
	t4 := now

	repo := &mockFeedRepository{
		followingMap: map[string][]string{
			userID: {friendID},
		},
		postsMap: map[string][]models.FeedPost{
			friendID: {
				{ID: "p1", AuthorID: friendID, Content: "Old post", IsPublic: true, CreatedAt: t1},
				{ID: "p3", AuthorID: friendID, Content: "Recent public post", IsPublic: true, CreatedAt: t3},
				{ID: "p-private", AuthorID: friendID, Content: "Friend's private post", IsPublic: false, CreatedAt: t4},
			},
			userID: {
				{ID: "p2", AuthorID: userID, Content: "My older post", IsPublic: true, CreatedAt: t2},
				{ID: "p4", AuthorID: userID, Content: "My newest private post", IsPublic: false, CreatedAt: t4},
			},
		},
	}

	svc := NewFeedService(repo)

	// Page 1: PageSize = 2
	res, err := svc.GetFeed(context.Background(), models.GetFeedQuery{
		UserID:   userID,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Friend's private post should be excluded. User's own private post should be included.
	// Eligible posts in chronological order descending:
	// 1. p4 (now)
	// 2. p3 (now - 1h)
	// 3. p2 (now - 2h)
	// 4. p1 (now - 3h)
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
	if res.NextCursor == "" {
		t.Errorf("expected non-empty NextCursor")
	}

	// Page 2: Fetch next page using NextCursor
	res2, err := svc.GetFeed(context.Background(), models.GetFeedQuery{
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
