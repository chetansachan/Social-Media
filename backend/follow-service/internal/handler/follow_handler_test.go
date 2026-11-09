package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/chetansachan/socialflow/follow-service/internal/models"
	"github.com/chetansachan/socialflow/follow-service/internal/services"
	followpb "github.com/chetansachan/socialflow/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeFollowService struct {
	services.FollowService

	// err is returned by every method, which is how the error-mapping table
	// drives all seven RPCs from one place.
	err error

	// Counts and status answers for the read RPCs.
	followers []string
	following []string
	status    bool

	lastFollow   [2]string
	lastUnfollow [2]string
	lastRemove   [2]string
	lastGet      string
	lastStatus   [2]string
}

func (s *fakeFollowService) FollowUser(ctx context.Context, followerID, followingID string) error {
	s.lastFollow = [2]string{followerID, followingID}
	return s.err
}

func (s *fakeFollowService) UnfollowUser(ctx context.Context, followerID, followingID string) error {
	s.lastUnfollow = [2]string{followerID, followingID}
	return s.err
}

func (s *fakeFollowService) RemoveFollower(ctx context.Context, userID, followerID string) error {
	s.lastRemove = [2]string{userID, followerID}
	return s.err
}

func (s *fakeFollowService) GetFollowers(ctx context.Context, userID string) ([]string, error) {
	s.lastGet = userID
	return s.followers, s.err
}

func (s *fakeFollowService) GetFollowing(ctx context.Context, userID string) ([]string, error) {
	s.lastGet = userID
	return s.following, s.err
}

func (s *fakeFollowService) GetFollowCounts(ctx context.Context, userID string) (*models.FollowCounts, error) {
	s.lastGet = userID
	if s.err != nil {
		return nil, s.err
	}
	return &models.FollowCounts{FollowersCount: 3, FollowingCount: 5}, nil
}

func (s *fakeFollowService) GetFollowStatus(ctx context.Context, followerID, followingID string) (bool, error) {
	s.lastStatus = [2]string{followerID, followingID}
	return s.status, s.err
}

func TestFollowUserForwardsBothIds(t *testing.T) {
	follower, following := uuid.New(), uuid.New()
	service := &fakeFollowService{}
	h := NewFollowHandler(service)

	res, err := h.FollowUser(context.Background(), &followpb.FollowUserRequest{
		FollowerId:  follower.String(),
		FollowingId: following.String(),
	})
	if err != nil {
		t.Fatalf("FollowUser: %v", err)
	}
	if service.lastFollow != [2]string{follower.String(), following.String()} {
		t.Fatalf("service saw %v", service.lastFollow)
	}
	if res.GetMessage() == "" {
		t.Fatal("expected a confirmation message")
	}
}

func TestFollowUserRejectsSelf(t *testing.T) {
	service := &fakeFollowService{err: models.ErrCannotFollowSelf}
	h := NewFollowHandler(service)
	id := uuid.New().String()

	_, err := h.FollowUser(context.Background(), &followpb.FollowUserRequest{FollowerId: id, FollowingId: id})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

// A nil request is a programming error, not a client error worth a 400 that
// hides a crash in the logs.
func TestHandlersRejectNilRequest(t *testing.T) {
	h := NewFollowHandler(&fakeFollowService{})

	if _, err := h.FollowUser(context.Background(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatal("nil follow request should be rejected")
	}
	if _, err := h.UnfollowUser(context.Background(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatal("nil unfollow request should be rejected")
	}
	if _, err := h.GetFollowers(context.Background(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatal("nil followers request should be rejected")
	}
	if _, err := h.GetFollowCounts(context.Background(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatal("nil counts request should be rejected")
	}
}

func TestWriteMutationErrorMapping(t *testing.T) {
	// Codes every write shares: an invalid id and an unexpected failure mean the
	// same thing whichever relationship asked.
	for _, tc := range []struct {
		name string
		err  error
		want codes.Code
	}{
		{"bad ids", models.ErrInvalidID, codes.InvalidArgument},
		{"unknown", errors.New("connection reset"), codes.Internal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewFollowHandler(&fakeFollowService{err: tc.err})
			id := uuid.New().String()

			_, err := h.FollowUser(context.Background(), &followpb.FollowUserRequest{FollowerId: id, FollowingId: uuid.New().String()})
			if status.Code(err) != tc.want {
				t.Fatalf("follow: code = %v, want %v", status.Code(err), tc.want)
			}

			_, err = h.UnfollowUser(context.Background(), &followpb.UnfollowUserRequest{FollowerId: id, FollowingId: uuid.New().String()})
			if status.Code(err) != tc.want {
				t.Fatalf("unfollow: code = %v, want %v", status.Code(err), tc.want)
			}

			_, err = h.RemoveFollower(context.Background(), &followpb.RemoveFollowerRequest{UserId: id, FollowerId: uuid.New().String()})
			if status.Code(err) != tc.want {
				t.Fatalf("remove: code = %v, want %v", status.Code(err), tc.want)
			}
		})
	}
}

// Follow-specific outcomes are only meaningful for FollowUser; treating them as
// shared would make an unfollow look like a client error it never was.
func TestFollowUserMapsItsOwnErrors(t *testing.T) {
	cases := []struct {
		err  error
		want codes.Code
	}{
		{models.ErrCannotFollowSelf, codes.InvalidArgument},
		{models.ErrAlreadyFollowing, codes.AlreadyExists},
	}
	for _, tc := range cases {
		h := NewFollowHandler(&fakeFollowService{err: tc.err})
		_, err := h.FollowUser(context.Background(), &followpb.FollowUserRequest{
			FollowerId:  uuid.New().String(),
			FollowingId: uuid.New().String(),
		})
		if status.Code(err) != tc.want {
			t.Fatalf("%v: code = %v, want %v", tc.err, status.Code(err), tc.want)
		}
	}
}

// Unfollowing a relationship that never existed is a missing resource, not an
// internal failure: the client is asking about something specific.
func TestUnfollowMapsMissingRelationshipToNotFound(t *testing.T) {
	h := NewFollowHandler(&fakeFollowService{err: models.ErrRelationNotFound})

	_, err := h.UnfollowUser(context.Background(), &followpb.UnfollowUserRequest{
		FollowerId:  uuid.New().String(),
		FollowingId: uuid.New().String(),
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
}

func TestReadRPCsForwardTheSubject(t *testing.T) {
	service := &fakeFollowService{
		followers: []string{uuid.New().String(), uuid.New().String()},
		following: []string{uuid.New().String()},
		status:    true,
	}
	h := NewFollowHandler(service)
	user := uuid.New().String()

	followers, err := h.GetFollowers(context.Background(), &followpb.GetFollowersRequest{UserId: user})
	if err != nil {
		t.Fatalf("GetFollowers: %v", err)
	}
	if service.lastGet != user {
		t.Fatalf("service looked up %q, want %q", service.lastGet, user)
	}
	if len(followers.GetFollowerIds()) != 2 {
		t.Fatalf("follower ids = %v", followers.GetFollowerIds())
	}

	following, err := h.GetFollowing(context.Background(), &followpb.GetFollowingRequest{UserId: user})
	if err != nil {
		t.Fatalf("GetFollowing: %v", err)
	}
	if len(following.GetFollowingIds()) != 1 {
		t.Fatalf("following ids = %v", following.GetFollowingIds())
	}

	counts, err := h.GetFollowCounts(context.Background(), &followpb.GetFollowCountsRequest{UserId: user})
	if err != nil {
		t.Fatalf("GetFollowCounts: %v", err)
	}
	if counts.GetFollowersCount() != 3 || counts.GetFollowingCount() != 5 {
		t.Fatalf("counts = %+v", counts)
	}

	follower := uuid.New().String()
	followingUser := uuid.New().String()
	isFollowing, err := h.GetFollowStatus(context.Background(), &followpb.GetFollowStatusRequest{
		FollowerId:  follower,
		FollowingId: followingUser,
	})
	if err != nil {
		t.Fatalf("GetFollowStatus: %v", err)
	}
	if service.lastStatus != [2]string{follower, followingUser} {
		t.Fatalf("service status args = %v", service.lastStatus)
	}
	if !isFollowing.GetIsFollowing() {
		t.Fatal("the follow button would render the wrong state")
	}
}
