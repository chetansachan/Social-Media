package handler

import (
	"context"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	"github.com/chetansachan/socialflow/feed-service/internal/services"
	feedpb "github.com/chetansachan/socialflow/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockFeedService struct {
	result *models.FeedResult
	err    error
}

func (m *mockFeedService) GetFeed(ctx context.Context, query models.GetFeedQuery) (*models.FeedResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func (m *mockFeedService) InvalidateUserFeed(ctx context.Context, userID string) error {
	return m.err
}

func (m *mockFeedService) Fanout() services.FanoutService {
	return nil
}

func TestFeedHandler_GetFeed_MissingUserID(t *testing.T) {
	h := NewFeedHandler(&mockFeedService{})
	_, err := h.GetFeed(context.Background(), &feedpb.GetFeedRequest{
		UserId: "",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument code, got %v", st.Code())
	}
}

func TestFeedHandler_GetFeed_Success(t *testing.T) {
	userID := uuid.New().String()
	postID := uuid.New().String()
	now := time.Now()

	mockSvc := &mockFeedService{
		result: &models.FeedResult{
			Posts: []models.FeedPost{
				{
					ID:        postID,
					AuthorID:  userID,
					Content:   "Hello from feed test",
					MediaURL:  "https://example.com/image.jpg",
					IsPublic:  true,
					LikeCount: 5,
					CreatedAt: now,
				},
			},
			NextCursor: "next-token",
			HasMore:    true,
		},
	}

	h := NewFeedHandler(mockSvc)
	resp, err := h.GetFeed(context.Background(), &feedpb.GetFeedRequest{
		UserId:   userID,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.GetPosts()) != 1 {
		t.Fatalf("expected 1 post, got %d", len(resp.GetPosts()))
	}
	if resp.GetPosts()[0].GetId() != postID {
		t.Errorf("expected post id %s, got %s", postID, resp.GetPosts()[0].GetId())
	}
	if resp.GetNextCursor() != "next-token" {
		t.Errorf("expected next cursor 'next-token', got %s", resp.GetNextCursor())
	}
	if !resp.GetHasMore() {
		t.Errorf("expected HasMore to be true")
	}
}

func TestFeedHandler_InvalidateUserFeed(t *testing.T) {
	userID := uuid.New().String()
	h := NewFeedHandler(&mockFeedService{})
	resp, err := h.InvalidateUserFeed(context.Background(), &feedpb.InvalidateUserFeedRequest{
		UserId: userID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetSuccess() {
		t.Errorf("expected success to be true")
	}
}
