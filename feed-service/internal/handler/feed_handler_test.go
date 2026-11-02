package handler

import (
	"context"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	feedpb "github.com/chetansachan/socialflow/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockFeedService struct {
	result *models.FeedResult
	err    error
}

func (m *mockFeedService) GetFeed(context.Context, models.GetFeedQuery) (*models.FeedResult, error) {
	return m.result, m.err
}

func (m *mockFeedService) InvalidateUserFeed(context.Context, string) error {
	return m.err
}

func TestGetFeed_MapsPostsAndCursor(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond)
	handler := NewFeedHandler(&mockFeedService{result: &models.FeedResult{
		Posts: []models.FeedPost{{
			ID:        uuid.New().String(),
			AuthorID:  uuid.New().String(),
			Content:   "hello",
			MediaURL:  "https://example.com/a.jpg",
			IsPublic:  true,
			LikeCount: 7,
			CreatedAt: now,
			UpdatedAt: &now,
		}},
		NextCursor: "next",
		HasMore:    true,
	}})

	resp, err := handler.GetFeed(context.Background(), &feedpb.GetFeedRequest{
		UserId: uuid.New().String(), PageSize: 20,
	})
	if err != nil {
		t.Fatalf("GetFeed: %v", err)
	}
	if len(resp.GetPosts()) != 1 || resp.GetPosts()[0].GetLikeCount() != 7 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.GetNextCursor() != "next" || !resp.GetHasMore() {
		t.Fatalf("pagination not mapped: %+v", resp)
	}
}

func TestGetFeed_RequiresUserID(t *testing.T) {
	handler := NewFeedHandler(&mockFeedService{})
	_, err := handler.GetFeed(context.Background(), &feedpb.GetFeedRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestGetFeed_MapsDomainErrors(t *testing.T) {
	handler := NewFeedHandler(&mockFeedService{err: models.ErrInvalidCursor})
	_, err := handler.GetFeed(context.Background(), &feedpb.GetFeedRequest{UserId: uuid.New().String()})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for bad cursor, got %v", err)
	}

	handler = NewFeedHandler(&mockFeedService{err: models.ErrFeedUnavailable})
	_, err = handler.GetFeed(context.Background(), &feedpb.GetFeedRequest{UserId: uuid.New().String()})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("expected Unavailable, got %v", err)
	}
}

func TestInvalidateUserFeed(t *testing.T) {
	handler := NewFeedHandler(&mockFeedService{})
	resp, err := handler.InvalidateUserFeed(context.Background(), &feedpb.InvalidateUserFeedRequest{
		UserId: uuid.New().String(),
	})
	if err != nil {
		t.Fatalf("InvalidateUserFeed: %v", err)
	}
	if !resp.GetSuccess() {
		t.Fatal("expected success=true")
	}

	_, err = handler.InvalidateUserFeed(context.Background(), &feedpb.InvalidateUserFeedRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}
