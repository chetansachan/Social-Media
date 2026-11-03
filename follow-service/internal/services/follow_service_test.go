package services

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/chetansachan/socialflow/follow-service/internal/models"
	"github.com/google/uuid"
)

type fakeFollowRepo struct {
	// follows[follower][following] = true
	follows map[uuid.UUID]map[uuid.UUID]bool
}

func newFakeFollowRepo() *fakeFollowRepo {
	return &fakeFollowRepo{follows: make(map[uuid.UUID]map[uuid.UUID]bool)}
}

func (r *fakeFollowRepo) FollowUser(ctx context.Context, followerID, followingID uuid.UUID) error {
	if r.follows[followerID] == nil {
		r.follows[followerID] = make(map[uuid.UUID]bool)
	}
	// Mirror the SQL upsert: repeating the call is a no-op.
	r.follows[followerID][followingID] = true
	return nil
}

func (r *fakeFollowRepo) UnfollowUser(ctx context.Context, followerID, followingID uuid.UUID) error {
	delete(r.follows[followerID], followingID)
	return nil
}

func (r *fakeFollowRepo) GetFollowers(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	followers := make([]uuid.UUID, 0)
	for followerID, following := range r.follows {
		if following[userID] {
			followers = append(followers, followerID)
		}
	}
	return followers, nil
}

func (r *fakeFollowRepo) GetFollowing(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	following := make([]uuid.UUID, 0, len(r.follows[userID]))
	for followingID := range r.follows[userID] {
		following = append(following, followingID)
	}
	return following, nil
}

func (r *fakeFollowRepo) GetFollowCounts(ctx context.Context, userID uuid.UUID) (*models.FollowCounts, error) {
	followers := 0
	for _, following := range r.follows {
		if following[userID] {
			followers++
		}
	}
	return &models.FollowCounts{
		FollowersCount: followers,
		FollowingCount: len(r.follows[userID]),
	}, nil
}

func (r *fakeFollowRepo) GetFollowStatus(ctx context.Context, followerID, followingID uuid.UUID) (bool, error) {
	return r.follows[followerID][followingID], nil
}

func (r *fakeFollowRepo) RemoveFollower(ctx context.Context, userID, followerID uuid.UUID) error {
	delete(r.follows[followerID], userID)
	return nil
}

func TestFollowUserSuccess(t *testing.T) {
	repo := newFakeFollowRepo()
	svc := NewFollowService(repo)
	follower, following := uuid.New(), uuid.New()

	if err := svc.FollowUser(context.Background(), follower.String(), following.String()); err != nil {
		t.Fatalf("FollowUser: %v", err)
	}
	if !repo.follows[follower][following] {
		t.Fatal("relationship was not recorded")
	}

	counts, err := svc.GetFollowCounts(context.Background(), following.String())
	if err != nil {
		t.Fatalf("GetFollowCounts: %v", err)
	}
	if counts.FollowersCount != 1 {
		t.Fatalf("followers = %d, want 1", counts.FollowersCount)
	}
}

func TestFollowUserCannotFollowSelf(t *testing.T) {
	svc := NewFollowService(newFakeFollowRepo())
	self := uuid.NewString()

	err := svc.FollowUser(context.Background(), self, self)
	if !errors.Is(err, models.ErrCannotFollowSelf) {
		t.Fatalf("err = %v, want ErrCannotFollowSelf", err)
	}
}

func TestFollowUserIsIdempotent(t *testing.T) {
	repo := newFakeFollowRepo()
	svc := NewFollowService(repo)
	follower, following := uuid.New(), uuid.New()

	for i := 0; i < 2; i++ {
		if err := svc.FollowUser(context.Background(), follower.String(), following.String()); err != nil {
			t.Fatalf("FollowUser call %d: %v", i+1, err)
		}
	}

	counts, err := svc.GetFollowCounts(context.Background(), following.String())
	if err != nil {
		t.Fatalf("GetFollowCounts: %v", err)
	}
	if counts.FollowersCount != 1 {
		t.Fatalf("followers = %d, want no duplicate row", counts.FollowersCount)
	}
}

func TestFollowUserRejectsInvalidIDs(t *testing.T) {
	svc := NewFollowService(newFakeFollowRepo())

	cases := [][2]string{
		{"", uuid.NewString()},
		{uuid.NewString(), ""},
		{"not-a-uuid", uuid.NewString()},
		{uuid.NewString(), "not-a-uuid"},
	}
	for _, tc := range cases {
		if err := svc.FollowUser(context.Background(), tc[0], tc[1]); !errors.Is(err, models.ErrInvalidID) {
			t.Fatalf("FollowUser(%q, %q) err = %v, want ErrInvalidID", tc[0], tc[1], err)
		}
	}
}

func TestUnfollowUserSuccess(t *testing.T) {
	repo := newFakeFollowRepo()
	svc := NewFollowService(repo)
	follower, following := uuid.New(), uuid.New()
	if err := svc.FollowUser(context.Background(), follower.String(), following.String()); err != nil {
		t.Fatalf("FollowUser: %v", err)
	}

	if err := svc.UnfollowUser(context.Background(), follower.String(), following.String()); err != nil {
		t.Fatalf("UnfollowUser: %v", err)
	}
	if repo.follows[follower][following] {
		t.Fatal("relationship was not removed")
	}

	counts, err := svc.GetFollowCounts(context.Background(), following.String())
	if err != nil {
		t.Fatalf("GetFollowCounts: %v", err)
	}
	if counts.FollowersCount != 0 {
		t.Fatalf("followers = %d, want 0", counts.FollowersCount)
	}
}

func TestUnfollowUserNotFollowingIsNoop(t *testing.T) {
	svc := NewFollowService(newFakeFollowRepo())

	if err := svc.UnfollowUser(context.Background(), uuid.NewString(), uuid.NewString()); err != nil {
		t.Fatalf("unfollowing a missing relationship should succeed: %v", err)
	}
}

func TestGetFollowersReturnsStrings(t *testing.T) {
	repo := newFakeFollowRepo()
	svc := NewFollowService(repo)
	target := uuid.New()
	first, second := uuid.New(), uuid.New()
	repo.follows[first] = map[uuid.UUID]bool{target: true}
	repo.follows[second] = map[uuid.UUID]bool{target: true}

	followers, err := svc.GetFollowers(context.Background(), target.String())
	if err != nil {
		t.Fatalf("GetFollowers: %v", err)
	}
	sort.Strings(followers)
	want := []string{first.String(), second.String()}
	sort.Strings(want)
	if len(followers) != 2 || followers[0] != want[0] || followers[1] != want[1] {
		t.Fatalf("followers = %v, want %v", followers, want)
	}
}

func TestGetFollowingReturnsStrings(t *testing.T) {
	repo := newFakeFollowRepo()
	svc := NewFollowService(repo)
	follower := uuid.New()
	target := uuid.New()
	repo.follows[follower] = map[uuid.UUID]bool{target: true}

	following, err := svc.GetFollowing(context.Background(), follower.String())
	if err != nil {
		t.Fatalf("GetFollowing: %v", err)
	}
	if len(following) != 1 || following[0] != target.String() {
		t.Fatalf("following = %v", following)
	}
}

func TestGetFollowCounts(t *testing.T) {
	repo := newFakeFollowRepo()
	svc := NewFollowService(repo)
	user := uuid.New()
	for i := 0; i < 3; i++ {
		repo.follows[uuid.New()] = map[uuid.UUID]bool{user: true}
	}
	repo.follows[user] = make(map[uuid.UUID]bool)
	for i := 0; i < 2; i++ {
		repo.follows[user][uuid.New()] = true
	}

	counts, err := svc.GetFollowCounts(context.Background(), user.String())
	if err != nil {
		t.Fatalf("GetFollowCounts: %v", err)
	}
	if counts.FollowersCount != 3 || counts.FollowingCount != 2 {
		t.Fatalf("counts = %+v", counts)
	}
}

func TestGetFollowStatus(t *testing.T) {
	repo := newFakeFollowRepo()
	svc := NewFollowService(repo)
	follower, following := uuid.New(), uuid.New()
	repo.follows[follower] = map[uuid.UUID]bool{following: true}

	status, err := svc.GetFollowStatus(context.Background(), follower.String(), following.String())
	if err != nil {
		t.Fatalf("GetFollowStatus: %v", err)
	}
	if !status {
		t.Fatal("expected a following relationship")
	}

	status, err = svc.GetFollowStatus(context.Background(), following.String(), follower.String())
	if err != nil {
		t.Fatalf("GetFollowStatus: %v", err)
	}
	if status {
		t.Fatal("reverse relationship should not exist")
	}
}

func TestRemoveFollower(t *testing.T) {
	repo := newFakeFollowRepo()
	svc := NewFollowService(repo)
	owner, follower := uuid.New(), uuid.New()
	repo.follows[follower] = map[uuid.UUID]bool{owner: true}

	if err := svc.RemoveFollower(context.Background(), owner.String(), follower.String()); err != nil {
		t.Fatalf("RemoveFollower: %v", err)
	}
	if repo.follows[follower][owner] {
		t.Fatal("follower was not removed")
	}
}

func TestRemoveFollowerRejectsInvalidIDs(t *testing.T) {
	svc := NewFollowService(newFakeFollowRepo())

	if err := svc.RemoveFollower(context.Background(), "not-a-uuid", uuid.NewString()); !errors.Is(err, models.ErrInvalidID) {
		t.Fatalf("err = %v, want ErrInvalidID", err)
	}
}
