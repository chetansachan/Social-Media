package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/post-service/internal/cache"
	"github.com/chetansachan/socialflow/post-service/internal/models"
	"github.com/google/uuid"
)

type likeKey struct {
	postID uuid.UUID
	userID uuid.UUID
}

type fakePostRepo struct {
	posts       map[uuid.UUID]*models.Post
	likes       map[likeKey]bool
	savedFiles  []string
	forceGetErr error

	mu           sync.Mutex
	getByIDCalls int
	getByIDHook  func()
}

func newFakePostRepo() *fakePostRepo {
	return &fakePostRepo{
		posts: make(map[uuid.UUID]*models.Post),
		likes: make(map[likeKey]bool),
	}
}

func (r *fakePostRepo) CreatePost(ctx context.Context, input *models.CreatePostInput) (*models.Post, error) {
	isPublic := true
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	}
	post := &models.Post{
		ID:        uuid.New(),
		AuthorID:  input.AuthorID,
		Content:   input.Content,
		MediaURL:  input.MediaURL,
		IsPublic:  isPublic,
		CreatedAt: time.Now().UTC(),
	}
	stored := *post
	r.posts[post.ID] = &stored
	return post, nil
}

func (r *fakePostRepo) GetPostById(ctx context.Context, id uuid.UUID) (*models.Post, error) {
	r.mu.Lock()
	r.getByIDCalls++
	hook := r.getByIDHook
	r.mu.Unlock()

	if hook != nil {
		hook()
	}

	if r.forceGetErr != nil {
		return nil, r.forceGetErr
	}
	post, ok := r.posts[id]
	if !ok || post.DeletedAt != nil {
		return nil, models.ErrPostNotFound
	}
	copy := *post
	return &copy, nil
}

func (r *fakePostRepo) getByIDCallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getByIDCalls
}

func (r *fakePostRepo) UpdatePost(ctx context.Context, input *models.UpdatePostInput, postID uuid.UUID, authorID uuid.UUID) (*models.Post, error) {
	post, ok := r.posts[postID]
	if !ok || post.DeletedAt != nil || post.AuthorID != authorID {
		return nil, models.ErrPostNotFound
	}
	if input.Content != nil {
		post.Content = *input.Content
	}
	if input.IsPublic != nil {
		post.IsPublic = *input.IsPublic
	}
	now := time.Now().UTC()
	post.UpdatedAt = &now
	copy := *post
	return &copy, nil
}

func (r *fakePostRepo) DeletePost(ctx context.Context, postID uuid.UUID, authorID uuid.UUID) error {
	post, ok := r.posts[postID]
	if !ok || post.AuthorID != authorID {
		return models.ErrPostNotFound
	}
	delete(r.posts, postID)
	return nil
}

func (r *fakePostRepo) MovePostToArchive(ctx context.Context, postID uuid.UUID, authorID uuid.UUID) error {
	post, ok := r.posts[postID]
	if !ok || post.DeletedAt != nil || post.AuthorID != authorID {
		return models.ErrPostNotFound
	}
	now := time.Now().UTC()
	post.DeletedAt = &now
	return nil
}

func (r *fakePostRepo) GetAllPost(ctx context.Context, authorID uuid.UUID) ([]models.Post, error) {
	posts := make([]models.Post, 0)
	for _, post := range r.posts {
		if post.AuthorID == authorID && post.DeletedAt == nil {
			posts = append(posts, *post)
		}
	}
	return posts, nil
}

func (r *fakePostRepo) GetAllArchivedPost(ctx context.Context, authorID uuid.UUID) ([]models.Post, error) {
	posts := make([]models.Post, 0)
	for _, post := range r.posts {
		if post.AuthorID == authorID && post.DeletedAt != nil {
			posts = append(posts, *post)
		}
	}
	return posts, nil
}

func (r *fakePostRepo) GetArchivedPostById(ctx context.Context, id uuid.UUID) (*models.Post, error) {
	post, ok := r.posts[id]
	if !ok || post.DeletedAt == nil {
		return nil, models.ErrPostNotFound
	}
	copy := *post
	return &copy, nil
}

func (r *fakePostRepo) LikePost(ctx context.Context, postID, userID uuid.UUID) (bool, error) {
	post, ok := r.posts[postID]
	if !ok || post.DeletedAt != nil {
		return false, models.ErrPostNotFound
	}

	key := likeKey{postID: postID, userID: userID}
	liked := !r.likes[key]
	r.likes[key] = liked
	if liked {
		post.LikeCount++
	} else if post.LikeCount > 0 {
		post.LikeCount--
	}
	return liked, nil
}

func (r *fakePostRepo) GetAllPostLikers(ctx context.Context, postID uuid.UUID) ([]string, error) {
	likers := make([]string, 0)
	for key, liked := range r.likes {
		if key.postID == postID && liked {
			likers = append(likers, key.userID.String())
		}
	}
	return likers, nil
}

func (r *fakePostRepo) GetAllLikedPosts(ctx context.Context, userID uuid.UUID) ([]models.Post, error) {
	posts := make([]models.Post, 0)
	for key, liked := range r.likes {
		if key.userID == userID && liked {
			if post, ok := r.posts[key.postID]; ok && post.DeletedAt == nil {
				posts = append(posts, *post)
			}
		}
	}
	return posts, nil
}

func (r *fakePostRepo) GetLikeStates(ctx context.Context, userID uuid.UUID, postIDs []uuid.UUID) (map[string]bool, error) {
	states := make(map[string]bool, len(postIDs))
	for _, postID := range postIDs {
		states[postID.String()] = r.likes[likeKey{postID: postID, userID: userID}]
	}
	return states, nil
}

func (r *fakePostRepo) SaveFiles(ctx context.Context, name string, ext string, mimeType string) error {
	r.savedFiles = append(r.savedFiles, name)
	return nil
}

func seedPost(repo *fakePostRepo, authorID uuid.UUID, content string) *models.Post {
	post := &models.Post{
		ID:        uuid.New(),
		AuthorID:  authorID,
		Content:   content,
		IsPublic:  true,
		CreatedAt: time.Now().UTC(),
	}
	repo.posts[post.ID] = post
	return post
}

func TestCreatePostSuccess(t *testing.T) {
	repo := newFakePostRepo()
	svc := NewPostService(repo)
	authorID := uuid.New()

	first, err := svc.CreatePost(context.Background(), models.CreatePostInput{
		AuthorID: authorID,
		Content:  "  first post  ",
	})
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if first.ID == uuid.Nil {
		t.Fatal("post has no id")
	}
	if first.Content != "first post" {
		t.Fatalf("content = %q, want trimmed text", first.Content)
	}
	if first.CreatedAt.IsZero() {
		t.Fatal("post has no created_at")
	}
	if !first.IsPublic {
		t.Fatal("is_public should default to true")
	}

	second, err := svc.CreatePost(context.Background(), models.CreatePostInput{
		AuthorID: authorID,
		Content:  "second post",
	})
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("two posts shared an id")
	}
}

func TestCreatePostHonorsExplicitVisibility(t *testing.T) {
	repo := newFakePostRepo()
	svc := NewPostService(repo)
	isPublic := false

	post, err := svc.CreatePost(context.Background(), models.CreatePostInput{
		AuthorID: uuid.New(),
		Content:  "private thoughts",
		IsPublic: &isPublic,
	})
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if post.IsPublic {
		t.Fatal("is_public = false was ignored")
	}
}

func TestCreatePostRejectsEmptyContent(t *testing.T) {
	svc := NewPostService(newFakePostRepo())

	for _, content := range []string{"", "   ", "\n\t"} {
		_, err := svc.CreatePost(context.Background(), models.CreatePostInput{
			AuthorID: uuid.New(),
			Content:  content,
		})
		if !errors.Is(err, models.ErrInvalidInput) {
			t.Fatalf("content %q: err = %v, want ErrInvalidInput", content, err)
		}
	}
}

func TestCreatePostRejectsMissingAuthor(t *testing.T) {
	svc := NewPostService(newFakePostRepo())

	if _, err := svc.CreatePost(context.Background(), models.CreatePostInput{
		Content: "orphan post",
	}); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreatePostRejectsOverlongContent(t *testing.T) {
	svc := NewPostService(newFakePostRepo())

	_, err := svc.CreatePost(context.Background(), models.CreatePostInput{
		AuthorID: uuid.New(),
		Content:  strings.Repeat("a", maxPostLength+1),
	})
	if !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}

	// A caption exactly at the limit is still allowed, counted in runes so
	// multi-byte characters do not inflate its length.
	if _, err := svc.CreatePost(context.Background(), models.CreatePostInput{
		AuthorID: uuid.New(),
		Content:  strings.Repeat("é", maxPostLength),
	}); err != nil {
		t.Fatalf("at-limit content was rejected: %v", err)
	}
}

func TestUpdatePostSuccess(t *testing.T) {
	repo := newFakePostRepo()
	svc := NewPostService(repo)
	authorID := uuid.New()
	post := seedPost(repo, authorID, "before")

	content := "after"
	isPublic := false
	updated, err := svc.UpdatePost(context.Background(), models.UpdatePostInput{
		Content:  &content,
		IsPublic: &isPublic,
	}, post.ID.String(), authorID.String())
	if err != nil {
		t.Fatalf("UpdatePost: %v", err)
	}

	if updated.Content != "after" || updated.IsPublic {
		t.Fatalf("updated = %+v", updated)
	}
	if updated.UpdatedAt == nil {
		t.Fatal("updated_at was not set")
	}
}

func TestUpdatePostForbidden(t *testing.T) {
	repo := newFakePostRepo()
	svc := NewPostService(repo)
	post := seedPost(repo, uuid.New(), "mine")

	content := "hijacked"
	_, err := svc.UpdatePost(context.Background(), models.UpdatePostInput{
		Content: &content,
	}, post.ID.String(), uuid.NewString())
	if !errors.Is(err, models.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if repo.posts[post.ID].Content != "mine" {
		t.Fatal("the post was modified by a non-owner")
	}
}

func TestUpdatePostRejectsEmptyTextWithoutMedia(t *testing.T) {
	repo := newFakePostRepo()
	svc := NewPostService(repo)
	authorID := uuid.New()
	post := seedPost(repo, authorID, "text only")

	empty := "   "
	if _, err := svc.UpdatePost(context.Background(), models.UpdatePostInput{
		Content: &empty,
	}, post.ID.String(), authorID.String()); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestUpdatePostAllowsEmptyTextWhenMediaExists(t *testing.T) {
	repo := newFakePostRepo()
	svc := NewPostService(repo)
	authorID := uuid.New()
	post := seedPost(repo, authorID, "caption")
	post.MediaURL = "https://res.cloudinary.com/demo/image/upload/photo.jpg"

	empty := ""
	updated, err := svc.UpdatePost(context.Background(), models.UpdatePostInput{
		Content: &empty,
	}, post.ID.String(), authorID.String())
	if err != nil {
		t.Fatalf("UpdatePost: %v", err)
	}
	if updated.Content != "" {
		t.Fatalf("content = %q", updated.Content)
	}
}

func TestUpdatePostRejectsOverlongContent(t *testing.T) {
	repo := newFakePostRepo()
	svc := NewPostService(repo)
	authorID := uuid.New()
	post := seedPost(repo, authorID, "short")

	content := strings.Repeat("a", maxPostLength+1)
	if _, err := svc.UpdatePost(context.Background(), models.UpdatePostInput{
		Content: &content,
	}, post.ID.String(), authorID.String()); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestDeletePostSuccess(t *testing.T) {
	repo := newFakePostRepo()
	svc := NewPostService(repo)
	authorID := uuid.New()
	post := seedPost(repo, authorID, "delete me")

	res, err := svc.DeletePost(context.Background(), post.ID.String(), authorID.String())
	if err != nil {
		t.Fatalf("DeletePost: %v", err)
	}
	if res.Message == "" {
		t.Fatal("expected a confirmation message")
	}
	if _, ok := repo.posts[post.ID]; ok {
		t.Fatal("post was not deleted")
	}
}

func TestDeletePostForbidden(t *testing.T) {
	repo := newFakePostRepo()
	svc := NewPostService(repo)
	post := seedPost(repo, uuid.New(), "mine")

	_, err := svc.DeletePost(context.Background(), post.ID.String(), uuid.NewString())
	if !errors.Is(err, models.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if _, ok := repo.posts[post.ID]; !ok {
		t.Fatal("a non-owner deleted the post")
	}
}

func TestDeletePostMissingPost(t *testing.T) {
	svc := NewPostService(newFakePostRepo())

	if _, err := svc.DeletePost(context.Background(), uuid.NewString(), uuid.NewString()); !errors.Is(err, models.ErrPostNotFound) {
		t.Fatalf("err = %v, want ErrPostNotFound", err)
	}
}

func TestArchivePostSuccessAndExclusion(t *testing.T) {
	repo := newFakePostRepo()
	svc := NewPostService(repo)
	authorID := uuid.New()
	post := seedPost(repo, authorID, "archive me")

	if _, err := svc.MovePostToArchive(context.Background(), post.ID.String(), authorID.String()); err != nil {
		t.Fatalf("MovePostToArchive: %v", err)
	}

	active, err := svc.GetAllPost(context.Background(), authorID.String())
	if err != nil {
		t.Fatalf("GetAllPost: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("archived post still appears in the active list: %+v", active)
	}

	archived, err := svc.GetAllArchivedPost(context.Background(), authorID.String())
	if err != nil {
		t.Fatalf("GetAllArchivedPost: %v", err)
	}
	if len(archived) != 1 || archived[0].ID != post.ID {
		t.Fatalf("archived = %+v", archived)
	}
}

func TestArchivePostForbidden(t *testing.T) {
	repo := newFakePostRepo()
	svc := NewPostService(repo)
	post := seedPost(repo, uuid.New(), "mine")

	_, err := svc.MovePostToArchive(context.Background(), post.ID.String(), uuid.NewString())
	if !errors.Is(err, models.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if repo.posts[post.ID].DeletedAt != nil {
		t.Fatal("a non-owner archived the post")
	}
}

func TestLikePostToggle(t *testing.T) {
	repo := newFakePostRepo()
	svc := NewPostService(repo)
	authorID := uuid.New()
	post := seedPost(repo, authorID, "like me")
	likerID := uuid.New()

	liked, err := svc.LikePost(context.Background(), post.ID.String(), likerID.String())
	if err != nil {
		t.Fatalf("LikePost: %v", err)
	}
	if !liked {
		t.Fatal("first like should set isLiked")
	}
	if repo.posts[post.ID].LikeCount != 1 {
		t.Fatalf("like count = %d, want 1", repo.posts[post.ID].LikeCount)
	}

	liked, err = svc.LikePost(context.Background(), post.ID.String(), likerID.String())
	if err != nil {
		t.Fatalf("LikePost: %v", err)
	}
	if liked {
		t.Fatal("second call should unlike")
	}
	if repo.posts[post.ID].LikeCount != 0 {
		t.Fatalf("like count = %d, want 0", repo.posts[post.ID].LikeCount)
	}
}

func TestLikePostMissingPost(t *testing.T) {
	svc := NewPostService(newFakePostRepo())

	if _, err := svc.LikePost(context.Background(), uuid.NewString(), uuid.NewString()); !errors.Is(err, models.ErrPostNotFound) {
		t.Fatalf("err = %v, want ErrPostNotFound", err)
	}
}

func TestGetLikeStatesSkipsMalformedIDs(t *testing.T) {
	repo := newFakePostRepo()
	svc := NewPostService(repo)
	userID := uuid.New()
	post := seedPost(repo, uuid.New(), "post")
	repo.likes[likeKey{postID: post.ID, userID: userID}] = true

	states, err := svc.GetLikeStates(context.Background(), userID.String(), []string{
		post.ID.String(),
		"not-a-uuid",
		post.ID.String(),
	})
	if err != nil {
		t.Fatalf("GetLikeStates: %v", err)
	}
	if len(states) != 1 || !states[post.ID.String()] {
		t.Fatalf("states = %v", states)
	}
}

type fakePostCache struct {
	mu      sync.Mutex
	posts   map[uuid.UUID]models.Post
	getErr  error
	deleted []uuid.UUID
}

func newFakePostCache() *fakePostCache {
	return &fakePostCache{posts: make(map[uuid.UUID]models.Post)}
}

func (c *fakePostCache) Get(ctx context.Context, id uuid.UUID) (*models.Post, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.getErr != nil {
		return nil, c.getErr
	}
	post, ok := c.posts[id]
	if !ok {
		return nil, cache.ErrMiss
	}
	copied := post
	return &copied, nil
}

func (c *fakePostCache) Set(ctx context.Context, post *models.Post, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.posts[post.ID] = *post
	return nil
}

func (c *fakePostCache) Delete(ctx context.Context, id uuid.UUID) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.posts, id)
	c.deleted = append(c.deleted, id)
	return nil
}

func (c *fakePostCache) contains(id uuid.UUID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.posts[id]
	return ok
}

func (c *fakePostCache) wasDeleted(id uuid.UUID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, deleted := range c.deleted {
		if deleted == id {
			return true
		}
	}
	return false
}

func newCachedPostService(t *testing.T) (*postService, *fakePostRepo, *fakePostCache) {
	t.Helper()

	repo := newFakePostRepo()
	postCache := newFakePostCache()
	svc := NewPostService(repo, WithPostCache(postCache, time.Minute)).(*postService)
	return svc, repo, postCache
}

func TestGetPostByIdServesCachedPostWithoutHittingTheRepository(t *testing.T) {
	svc, repo, postCache := newCachedPostService(t)
	post := seedPost(repo, uuid.New(), "cached")
	postCache.posts[post.ID] = *post

	found, err := svc.GetPostById(context.Background(), post.ID.String())
	if err != nil {
		t.Fatalf("GetPostById: %v", err)
	}
	if found.ID != post.ID || found.Content != post.Content {
		t.Fatalf("found = %+v", found)
	}
	if calls := repo.getByIDCallCount(); calls != 0 {
		t.Fatalf("repository was queried %d times, want a cache hit", calls)
	}
}

func TestGetPostByIdCollapsesConcurrentMisses(t *testing.T) {
	svc, repo, postCache := newCachedPostService(t)
	post := seedPost(repo, uuid.New(), "viral")

	// Hold the first query long enough for every caller to join the group.
	repo.getByIDHook = func() { time.Sleep(50 * time.Millisecond) }

	const callers = 64
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.GetPostById(context.Background(), post.ID.String()); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("GetPostById: %v", err)
	}

	if calls := repo.getByIDCallCount(); calls != 1 {
		t.Fatalf("repository was queried %d times, want a single shared load", calls)
	}
	if !postCache.contains(post.ID) {
		t.Fatal("the shared load did not populate the cache")
	}
}

func TestGetPostByIdFallsBackWhenCacheIsUnavailable(t *testing.T) {
	svc, repo, postCache := newCachedPostService(t)
	post := seedPost(repo, uuid.New(), "fallback")
	postCache.getErr = errors.New("redis down")

	found, err := svc.GetPostById(context.Background(), post.ID.String())
	if err != nil {
		t.Fatalf("GetPostById: %v", err)
	}
	if found.ID != post.ID {
		t.Fatal("did not fall back to the repository")
	}
}

func TestUpdatePostInvalidatesCachedDetail(t *testing.T) {
	svc, repo, postCache := newCachedPostService(t)
	authorID := uuid.New()
	post := seedPost(repo, authorID, "before")
	postCache.posts[post.ID] = *post

	content := "after"
	if _, err := svc.UpdatePost(context.Background(), models.UpdatePostInput{Content: &content}, post.ID.String(), authorID.String()); err != nil {
		t.Fatalf("UpdatePost: %v", err)
	}
	if !postCache.wasDeleted(post.ID) {
		t.Fatal("update left a stale post in the cache")
	}
}

func TestDeletePostInvalidatesCachedDetail(t *testing.T) {
	svc, repo, postCache := newCachedPostService(t)
	authorID := uuid.New()
	post := seedPost(repo, authorID, "delete me")
	postCache.posts[post.ID] = *post

	if _, err := svc.DeletePost(context.Background(), post.ID.String(), authorID.String()); err != nil {
		t.Fatalf("DeletePost: %v", err)
	}
	if !postCache.wasDeleted(post.ID) {
		t.Fatal("delete left the post in the cache")
	}
}

func TestArchivePostInvalidatesCachedDetail(t *testing.T) {
	svc, repo, postCache := newCachedPostService(t)
	authorID := uuid.New()
	post := seedPost(repo, authorID, "archive me")
	postCache.posts[post.ID] = *post

	if _, err := svc.MovePostToArchive(context.Background(), post.ID.String(), authorID.String()); err != nil {
		t.Fatalf("MovePostToArchive: %v", err)
	}
	if !postCache.wasDeleted(post.ID) {
		t.Fatal("archive left the post in the cache")
	}
}

func TestLikePostInvalidatesCachedDetail(t *testing.T) {
	svc, repo, postCache := newCachedPostService(t)
	post := seedPost(repo, uuid.New(), "like me")
	postCache.posts[post.ID] = *post

	if _, err := svc.LikePost(context.Background(), post.ID.String(), uuid.NewString()); err != nil {
		t.Fatalf("LikePost: %v", err)
	}
	if !postCache.wasDeleted(post.ID) {
		t.Fatal("like left a stale like_count in the cache")
	}
}
