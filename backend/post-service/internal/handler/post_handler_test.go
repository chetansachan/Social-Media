package handler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/post-service/internal/models"
	postpb "github.com/chetansachan/socialflow/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakePostService is a hand-written stand-in for services.PostService. It keeps
// every test hermetic: no repository, no Redis, no Cloudinary, so a test only
// exercises the handler's translation between protobuf and domain types.
type fakePostService struct {
	post      *models.Post
	posts     []models.Post
	signature *models.UploadSignature
	response  *models.Response
	isLiked   bool
	likers    []string
	liked     map[string]bool

	// err is returned by every method, which lets a test drive a single failure
	// mode without replaying whole call sequences.
	err error

	lastCreate models.CreatePostInput
	lastGet    string
	lastAll    string
	lastDelete struct {
		postID   string
		authorID string
	}
	lastSearch struct {
		query string
		limit int32
	}
	lastSignature struct {
		userID       string
		resourceType string
		folder       string
	}
}

func (s *fakePostService) GetPostById(ctx context.Context, id string) (*models.Post, error) {
	s.lastGet = id
	return s.post, s.err
}

func (s *fakePostService) GetAllPost(ctx context.Context, authorId string) ([]models.Post, error) {
	s.lastAll = authorId
	return s.posts, s.err
}

func (s *fakePostService) CreatePost(ctx context.Context, input models.CreatePostInput) (*models.Post, error) {
	s.lastCreate = input
	return s.post, s.err
}

func (s *fakePostService) UpdatePost(ctx context.Context, input models.UpdatePostInput, postId string, authorId string) (*models.Post, error) {
	return s.post, s.err
}

func (s *fakePostService) DeletePost(ctx context.Context, postId string, authorId string) (*models.Response, error) {
	s.lastDelete.postID = postId
	s.lastDelete.authorID = authorId
	return s.response, s.err
}

func (s *fakePostService) MovePostToArchive(ctx context.Context, postId string, authorId string) (*models.Response, error) {
	return s.response, s.err
}

func (s *fakePostService) GetAllArchivedPost(ctx context.Context, authorId string) ([]models.Post, error) {
	s.lastAll = authorId
	return s.posts, s.err
}

func (s *fakePostService) GetArchivedPostById(ctx context.Context, id string) (*models.Post, error) {
	s.lastGet = id
	return s.post, s.err
}

func (s *fakePostService) LikePost(ctx context.Context, postId string, userId string) (bool, error) {
	return s.isLiked, s.err
}

func (s *fakePostService) GetAllPostLikers(ctx context.Context, postId string) ([]string, error) {
	return s.likers, s.err
}

func (s *fakePostService) GetAllLikedPosts(ctx context.Context, userId string) ([]models.Post, error) {
	return s.posts, s.err
}

func (s *fakePostService) GetLikeStates(ctx context.Context, userId string, postIds []string) (map[string]bool, error) {
	return s.liked, s.err
}

func (s *fakePostService) SearchPosts(ctx context.Context, query string, limit int32) ([]models.Post, error) {
	s.lastSearch.query = query
	s.lastSearch.limit = limit
	return s.posts, s.err
}

func (s *fakePostService) Upload(ctx context.Context, base64 string) (string, error) {
	return "", s.err
}

func (s *fakePostService) CreateUploadSignature(ctx context.Context, userId, resourceType, folder string) (*models.UploadSignature, error) {
	s.lastSignature.userID = userId
	s.lastSignature.resourceType = resourceType
	s.lastSignature.folder = folder
	return s.signature, s.err
}

// samplePost uses fixed IDs and timestamps so a failure prints a readable diff
// instead of random UUIDs.
func samplePost() *models.Post {
	created := time.Date(2026, 11, 7, 11, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	return &models.Post{
		ID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		AuthorID:  uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Content:   "hello world",
		MediaURL:  "https://res.cloudinary.com/demo/image/upload/cat.jpg",
		IsPublic:  true,
		LikeCount: 7,
		CreatedAt: created,
		UpdatedAt: &updated,
	}
}

func TestCreatePostMapsServiceResult(t *testing.T) {
	post := samplePost()
	isPublic := true
	service := &fakePostService{post: post}
	h := NewPostHandler(service)

	resp, err := h.CreatePost(context.Background(), &postpb.CreatePostRequest{
		AuthorId: post.AuthorID.String(),
		Content:  post.Content,
		MediaUrl: post.MediaURL,
		IsPublic: &isPublic,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got := resp.GetPost()
	if got == nil {
		t.Fatal("response has no post")
	}
	// UUIDs and timestamps become strings on the wire, so the handler has to
	// stringify them rather than forward the domain values.
	if got.GetId() != post.ID.String() || got.GetAuthorId() != post.AuthorID.String() {
		t.Fatalf("ids = %q/%q, want %q/%q", got.GetId(), got.GetAuthorId(), post.ID, post.AuthorID)
	}
	if got.GetContent() != post.Content || got.GetMediaUrl() != post.MediaURL {
		t.Fatalf("post = %+v, want content %q media %q", got, post.Content, post.MediaURL)
	}
	if got.GetIsPublic() != post.IsPublic {
		t.Fatalf("is_public = %v, want %v", got.GetIsPublic(), post.IsPublic)
	}
	// LikeCount widens from int to int32, the only lossy conversion in the
	// mapping and therefore worth pinning down.
	if got.GetLikeCount() != int32(post.LikeCount) {
		t.Fatalf("like_count = %d, want %d", got.GetLikeCount(), post.LikeCount)
	}
	if got.GetCreatedAt() != post.CreatedAt.Format(time.RFC3339) {
		t.Fatalf("created_at = %q, want %q", got.GetCreatedAt(), post.CreatedAt.Format(time.RFC3339))
	}
	if got.GetUpdatedAt() != post.UpdatedAt.Format(time.RFC3339) {
		t.Fatalf("updated_at = %q, want %q", got.GetUpdatedAt(), post.UpdatedAt.Format(time.RFC3339))
	}

	// The service must see the parsed UUID, not the raw request string, or its
	// own validation becomes unreachable.
	if service.lastCreate.AuthorID != post.AuthorID {
		t.Fatalf("service received author %v, want %v", service.lastCreate.AuthorID, post.AuthorID)
	}
	if service.lastCreate.Content != post.Content || service.lastCreate.MediaURL != post.MediaURL {
		t.Fatalf("service received %+v", service.lastCreate)
	}
	if service.lastCreate.IsPublic == nil || *service.lastCreate.IsPublic != isPublic {
		t.Fatalf("service received is_public = %v, want %v", service.lastCreate.IsPublic, isPublic)
	}

	// Omitting the flag must forward nil rather than false: the service, not the
	// handler, owns the default visibility so the rule stays in one place.
	if _, err := h.CreatePost(context.Background(), &postpb.CreatePostRequest{
		AuthorId: post.AuthorID.String(),
		Content:  post.Content,
	}); err != nil {
		t.Fatalf("create without is_public: %v", err)
	}
	if service.lastCreate.IsPublic != nil {
		t.Fatalf("service received is_public = %v, want nil", *service.lastCreate.IsPublic)
	}
}

func TestCreatePostRejectsMalformedAuthorID(t *testing.T) {
	service := &fakePostService{post: samplePost()}
	h := NewPostHandler(service)

	_, err := h.CreatePost(context.Background(), &postpb.CreatePostRequest{
		AuthorId: "not-a-uuid",
		Content:  "hello",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
	// The author ID is parsed before the service is called, so a malformed one
	// must fail without leaving a row waiting behind it.
	if service.lastCreate != (models.CreatePostInput{}) {
		t.Fatalf("service was called with %+v", service.lastCreate)
	}
}

func TestCreatePostMapsServiceErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		// A rejected caption is the caller's fault, so it must not surface as a
		// 500 that a gateway would be tempted to retry.
		{"invalid input", models.ErrInvalidInput, codes.InvalidArgument},
		// Anything the handler does not recognise stays Internal, and the
		// underlying cause must survive so the failure is still diagnosable.
		{"internal", errors.New("insert post: connection reset"), codes.Internal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewPostHandler(&fakePostService{err: tc.err})

			resp, err := h.CreatePost(context.Background(), &postpb.CreatePostRequest{
				AuthorId: uuid.NewString(),
				Content:  "hello",
			})
			if err == nil {
				t.Fatalf("create succeeded with %+v", resp)
			}
			if status.Code(err) != tc.want {
				t.Fatalf("code = %v, want %v", status.Code(err), tc.want)
			}
			if tc.want != codes.Internal {
				return
			}
			if !strings.Contains(err.Error(), "connection reset") {
				t.Fatalf("message = %q, want the underlying cause", err)
			}
		})
	}
}

func TestGetPostByIdMapsPostFields(t *testing.T) {
	post := samplePost()
	h := NewPostHandler(&fakePostService{post: post})

	resp, err := h.GetPostById(context.Background(), &postpb.PostByIdRequest{Id: post.ID.String()})
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	got := resp.GetPost()
	if got.GetId() != post.ID.String() || got.GetAuthorId() != post.AuthorID.String() {
		t.Fatalf("ids = %q/%q, want %q/%q", got.GetId(), got.GetAuthorId(), post.ID, post.AuthorID)
	}
	if got.GetContent() != post.Content || got.GetMediaUrl() != post.MediaURL || !got.GetIsPublic() {
		t.Fatalf("post = %+v, want content %q media %q public %v", got, post.Content, post.MediaURL, post.IsPublic)
	}
	if got.GetLikeCount() != int32(post.LikeCount) {
		t.Fatalf("like_count = %d, want %d", got.GetLikeCount(), post.LikeCount)
	}
	// Timestamps cross the wire as RFC3339 strings, so the comparison is against
	// the formatted domain value rather than the raw time.
	if got.GetCreatedAt() != post.CreatedAt.Format(time.RFC3339) {
		t.Fatalf("created_at = %q, want %q", got.GetCreatedAt(), post.CreatedAt.Format(time.RFC3339))
	}
	if got.GetUpdatedAt() != post.UpdatedAt.Format(time.RFC3339) {
		t.Fatalf("updated_at = %q, want %q", got.GetUpdatedAt(), post.UpdatedAt.Format(time.RFC3339))
	}
}

func TestGetPostByIdEmptyUpdatedAtWhenNil(t *testing.T) {
	post := samplePost()
	post.UpdatedAt = nil

	h := NewPostHandler(&fakePostService{post: post})

	resp, err := h.GetPostById(context.Background(), &postpb.PostByIdRequest{Id: post.ID.String()})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// A post that was never edited has no updated_at, and the proto field is a
	// plain string, so the handler has to emit "" instead of the zero time.
	if got := resp.GetPost().GetUpdatedAt(); got != "" {
		t.Fatalf("updated_at = %q, want empty", got)
	}
}

func TestGetPostByIdMapsNotFound(t *testing.T) {
	h := NewPostHandler(&fakePostService{err: models.ErrPostNotFound})

	if _, err := h.GetPostById(context.Background(), &postpb.PostByIdRequest{
		Id: uuid.NewString(),
	}); status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
}

func TestGetAllPostsMapsList(t *testing.T) {
	authorID := uuid.New()
	posts := []models.Post{*samplePost(), *samplePost()}
	posts[1].Content = "second"
	posts[1].AuthorID = authorID

	service := &fakePostService{posts: posts}
	h := NewPostHandler(service)

	resp, err := h.GetAllPosts(context.Background(), &postpb.AllPostRequest{AuthorId: authorID.String()})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	got := resp.GetPost()
	if len(got) != len(posts) {
		t.Fatalf("len = %d, want %d", len(got), len(posts))
	}
	// Order has to survive the mapping because the repository already ranked the
	// feed.
	for i, p := range got {
		if p.GetId() != posts[i].ID.String() || p.GetContent() != posts[i].Content {
			t.Fatalf("post[%d] = %+v, want id %q content %q", i, p, posts[i].ID, posts[i].Content)
		}
		if p.GetAuthorId() != posts[i].AuthorID.String() {
			t.Fatalf("post[%d] author = %q, want %q", i, p.GetAuthorId(), posts[i].AuthorID)
		}
		if p.GetCreatedAt() != posts[i].CreatedAt.Format(time.RFC3339) {
			t.Fatalf("post[%d] created_at = %q", i, p.GetCreatedAt())
		}
	}
	// The author comes straight off the request so the service can page by it.
	if service.lastAll != authorID.String() {
		t.Fatalf("service received author %q, want %q", service.lastAll, authorID)
	}
}

func TestGetAllPostsReturnsEmptySlice(t *testing.T) {
	h := NewPostHandler(&fakePostService{})

	resp, err := h.GetAllPosts(context.Background(), &postpb.AllPostRequest{
		AuthorId: uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// An empty feed must serialise as [] and not null, so the slice has to be
	// allocated before the loop rather than left nil.
	if resp.GetPost() == nil {
		t.Fatal("posts = nil, want empty slice")
	}
	if len(resp.GetPost()) != 0 {
		t.Fatalf("posts = %v, want empty", resp.GetPost())
	}
}

func TestGetUploadSignatureMapsEveryField(t *testing.T) {
	signature := &models.UploadSignature{
		CloudName: "demo",
		APIKey:    "1234567890",
		Timestamp: 1762500000,
		Signature: "abc123",
		Folder:    "socialflow/user",
		PublicID:  uuid.NewString(),
		UploadURL: "https://api.cloudinary.com/v1_1/demo/auto/upload",
		ExpiresAt: 1762500600,
	}
	service := &fakePostService{signature: signature}
	h := NewPostHandler(service)

	resp, err := h.GetUploadSignature(context.Background(), &postpb.GetUploadSignatureRequest{
		UserId:       uuid.NewString(),
		ResourceType: "image",
		Folder:       signature.Folder,
	})
	if err != nil {
		t.Fatalf("signature: %v", err)
	}

	// The signature is time-boxed: expires_at is what lets a client refresh
	// before Cloudinary starts rejecting its PUT.
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"cloud_name", resp.GetCloudName(), signature.CloudName},
		{"api_key", resp.GetApiKey(), signature.APIKey},
		{"timestamp", resp.GetTimestamp(), signature.Timestamp},
		{"signature", resp.GetSignature(), signature.Signature},
		{"folder", resp.GetFolder(), signature.Folder},
		{"public_id", resp.GetPublicId(), signature.PublicID},
		{"upload_url", resp.GetUploadUrl(), signature.UploadURL},
		{"expires_at", resp.GetExpiresAt(), signature.ExpiresAt},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Fatalf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}

	// Resource type and folder must reach the service untouched; the handler
	// signs, it does not decide where the asset lands.
	if service.lastSignature.userID == "" || service.lastSignature.resourceType != "image" ||
		service.lastSignature.folder != signature.Folder {
		t.Fatalf("service received %+v", service.lastSignature)
	}
}

func TestGetUploadSignatureMapsErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		// An unsupported resource type is caller error.
		{"invalid input", models.ErrInvalidInput, codes.InvalidArgument},
		// Signing failures are configuration problems, not caller fault: the
		// request is well formed, the server just cannot honour it yet.
		{"not configured", errors.New("cloudinary is not configured"), codes.FailedPrecondition},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewPostHandler(&fakePostService{err: tc.err})

			_, err := h.GetUploadSignature(context.Background(), &postpb.GetUploadSignatureRequest{
				UserId:       uuid.NewString(),
				ResourceType: "image",
			})
			if status.Code(err) != tc.want {
				t.Fatalf("code = %v, want %v", status.Code(err), tc.want)
			}
		})
	}
}

func TestSearchPostsMapsResults(t *testing.T) {
	first, second := samplePost(), samplePost()
	second.Content = "#golang"
	service := &fakePostService{posts: []models.Post{*first, *second}}
	h := NewPostHandler(service)

	resp, err := h.SearchPosts(context.Background(), &postpb.SearchPostsRequest{
		Query: "#golang",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	got := resp.GetPosts()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// Every hit goes through toPostPB, so search results must carry the same ID
	// and timestamp formatting as the single-post RPC.
	for i, p := range got {
		if p.GetContent() != service.posts[i].Content || p.GetAuthorId() != service.posts[i].AuthorID.String() {
			t.Fatalf("post[%d] = %+v, want %+v", i, p, service.posts[i])
		}
		if p.GetCreatedAt() != service.posts[i].CreatedAt.Format(time.RFC3339) {
			t.Fatalf("post[%d] created_at = %q", i, p.GetCreatedAt())
		}
	}
	// The handler forwards the caller's limit verbatim; clamping happens in the
	// service so every transport gets the same page bound.
	if service.lastSearch.query != "#golang" || service.lastSearch.limit != 10 {
		t.Fatalf("service received %+v", service.lastSearch)
	}
}

func TestSearchPostsMapsInvalidInput(t *testing.T) {
	h := NewPostHandler(&fakePostService{err: models.ErrInvalidInput})

	_, err := h.SearchPosts(context.Background(), &postpb.SearchPostsRequest{Query: "   ", Limit: 5})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestToPostPBHandlesNilAndOptionalUpdatedAt(t *testing.T) {
	// Callers hand over rows straight from the repository, where a nil post is a
	// legitimate "no row" answer that must not panic the mapper.
	if got := toPostPB(nil); got != nil {
		t.Fatalf("toPostPB(nil) = %+v, want nil", got)
	}

	post := samplePost()
	pb := toPostPB(post)
	if pb.GetId() != post.ID.String() || pb.GetAuthorId() != post.AuthorID.String() {
		t.Fatalf("mapped ids = %q/%q", pb.GetId(), pb.GetAuthorId())
	}
	if pb.GetContent() != post.Content || pb.GetLikeCount() != int32(post.LikeCount) {
		t.Fatalf("mapped = %+v", pb)
	}
	if pb.GetCreatedAt() != post.CreatedAt.Format(time.RFC3339) {
		t.Fatalf("created_at = %q, want RFC3339", pb.GetCreatedAt())
	}
	if pb.GetUpdatedAt() != post.UpdatedAt.Format(time.RFC3339) {
		t.Fatalf("updated_at = %q, want RFC3339", pb.GetUpdatedAt())
	}

	// An unedited post has a nil UpdatedAt, and the proto field is a plain
	// string, so the mapper has to choose "" over the zero time.
	post.UpdatedAt = nil
	if got := toPostPB(post).GetUpdatedAt(); got != "" {
		t.Fatalf("updated_at = %q, want empty", got)
	}
}

// TestPostHandlerMapsDomainErrors pins the whole sentinel-to-status table in one
// place. Comment-domain sentinels deliberately land on Internal: that mapping
// belongs to the comment handler, and a post RPC must not report a comment
// failure as a client fault it does not own.
func TestPostHandlerMapsDomainErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		rpc  func(*PostHandler) error
		want codes.Code
	}{
		{
			name: "missing post",
			err:  models.ErrPostNotFound,
			rpc: func(h *PostHandler) error {
				_, err := h.GetPostById(context.Background(), &postpb.PostByIdRequest{Id: uuid.NewString()})
				return err
			},
			want: codes.NotFound,
		},
		{
			name: "rejected search",
			err:  models.ErrInvalidInput,
			rpc: func(h *PostHandler) error {
				_, err := h.SearchPosts(context.Background(), &postpb.SearchPostsRequest{Query: "#golang"})
				return err
			},
			want: codes.InvalidArgument,
		},
		{
			name: "not the author",
			err:  models.ErrUnauthorized,
			rpc: func(h *PostHandler) error {
				_, err := h.DeletePost(context.Background(), &postpb.DeletePostRequest{
					PostId:   uuid.NewString(),
					AuthorId: uuid.NewString(),
				})
				return err
			},
			want: codes.PermissionDenied,
		},
		{
			// A comment sentinel reaching a post RPC is an unmapped failure,
			// not a NotFound the client should act on.
			name: "comment sentinel is not a post sentinel",
			err:  models.ErrCommentNotFound,
			rpc: func(h *PostHandler) error {
				_, err := h.GetPostById(context.Background(), &postpb.PostByIdRequest{Id: uuid.NewString()})
				return err
			},
			want: codes.Internal,
		},
		{
			name: "generic failure",
			err:  errors.New("connection reset"),
			rpc: func(h *PostHandler) error {
				_, err := h.GetPostById(context.Background(), &postpb.PostByIdRequest{Id: uuid.NewString()})
				return err
			},
			want: codes.Internal,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewPostHandler(&fakePostService{err: tc.err})

			if err := tc.rpc(h); status.Code(err) != tc.want {
				t.Fatalf("code = %v, want %v", status.Code(err), tc.want)
			}
		})
	}
}
