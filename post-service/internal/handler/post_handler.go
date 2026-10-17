package handler

import (
	"context"
	"errors"
	"time"

	"github.com/chetansachan/socialflow/post-service/internal/models"
	"github.com/chetansachan/socialflow/post-service/internal/services"
	postpb "github.com/chetansachan/socialflow/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PostHandler struct {
	postpb.UnimplementedPostServiceServer
	postService services.PostService
}

func NewPostHandler(postService services.PostService) *PostHandler {
	return &PostHandler{postService: postService}
}

func formatTimestamp(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func (h *PostHandler) CreatePost(ctx context.Context, req *postpb.CreatePostRequest) (*postpb.CreatePostResponse, error) {
	authorId, err := uuid.Parse(req.AuthorId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid author ID format: %v", err)
	}

	input := models.CreatePostInput{
		AuthorID: authorId,
		Content:  req.Content,
		MediaURL: req.MediaUrl,
		IsPublic: req.IsPublic,
	}

	post, err := h.postService.CreatePost(ctx, input)
	if err != nil {
		if errors.Is(err, models.ErrInvalidInput) {
			return nil, status.Error(codes.InvalidArgument, "Invalid Input")
		}

		return nil, status.Errorf(codes.Internal, "failed to create post: %v", err)
	}

	return &postpb.CreatePostResponse{
		Post: &postpb.Post{
			Id:        post.ID.String(),
			AuthorId:  post.AuthorID.String(),
			Content:   post.Content,
			MediaUrl:  post.MediaURL,
			IsPublic:  post.IsPublic,
			LikeCount: int32(post.LikeCount),
			CreatedAt: post.CreatedAt.Format(time.RFC3339),
			UpdatedAt: formatTimestamp(post.UpdatedAt),
		},
	}, nil
}

func (h *PostHandler) GetPostById(ctx context.Context, input *postpb.PostByIdRequest) (*postpb.PostByIdResponse, error) {
	res, err := h.postService.GetPostById(ctx, input.Id)
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return &postpb.PostByIdResponse{}, status.Error(codes.NotFound, "Post Not Found")
		}
		return nil, status.Errorf(codes.Internal, "failed to find post %v", err)
	}

	return &postpb.PostByIdResponse{
		Post: &postpb.Post{
			Id:        res.ID.String(),
			AuthorId:  res.AuthorID.String(),
			Content:   res.Content,
			MediaUrl:  res.MediaURL,
			IsPublic:  res.IsPublic,
			LikeCount: int32(res.LikeCount),
			CreatedAt: res.CreatedAt.Format(time.RFC3339),
			UpdatedAt: formatTimestamp(res.UpdatedAt),
		},
	}, nil
}

func (h *PostHandler) GetAllPosts(ctx context.Context, input *postpb.AllPostRequest) (*postpb.AllPostResponse, error) {
	posts, err := h.postService.GetAllPost(ctx, input.AuthorId)
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return &postpb.AllPostResponse{}, status.Error(codes.NotFound, "Post Not Found")
		}
		return nil, status.Errorf(codes.Internal, "failed to find post %v", err)
	}

	Posts := make([]*postpb.Post, 0, len(posts))

	for _, res := range posts {
		Posts = append(Posts, &postpb.Post{
			Id:        res.ID.String(),
			AuthorId:  res.AuthorID.String(),
			Content:   res.Content,
			MediaUrl:  res.MediaURL,
			IsPublic:  res.IsPublic,
			LikeCount: int32(res.LikeCount),
			CreatedAt: res.CreatedAt.Format(time.RFC3339),
			UpdatedAt: formatTimestamp(res.UpdatedAt),
		})
	}

	return &postpb.AllPostResponse{
		Post: Posts,
	}, nil
}

func (h *PostHandler) DeletePost(ctx context.Context, input *postpb.DeletePostRequest) (*postpb.DeletePostResponse, error) {
	_, err := h.postService.DeletePost(ctx, input.PostId)
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return nil, status.Error(codes.NotFound, "Post Not Found")
		}

		return &postpb.DeletePostResponse{}, status.Errorf(codes.Internal, "Error Deleting Post: %v", err)
	}

	return &postpb.DeletePostResponse{
		Message: "Post Deleted Successfully",
	}, nil
}

func (h *PostHandler) UpdatePost(ctx context.Context, req *postpb.UpdatePostRequest) (*postpb.UpdatePostResponse, error) {
	input := models.UpdatePostInput{
		Content:  req.Content,
		IsPublic: req.IsPublic,
	}

	post, err := h.postService.UpdatePost(ctx, input, req.PostId, req.AuthorId)
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return nil, status.Error(codes.NotFound, "Post Not Found")
		}

		return nil, status.Errorf(codes.Internal, "failed to update post: %v", err)
	}

	return &postpb.UpdatePostResponse{
		Post: &postpb.Post{
			Id:        post.ID.String(),
			AuthorId:  post.AuthorID.String(),
			Content:   post.Content,
			MediaUrl:  post.MediaURL,
			IsPublic:  post.IsPublic,
			LikeCount: int32(post.LikeCount),
			CreatedAt: post.CreatedAt.Format(time.RFC3339),
			UpdatedAt: formatTimestamp(post.UpdatedAt),
		},
	}, nil
}

func (h *PostHandler) GetAllArchivedPosts(ctx context.Context, input *postpb.AllArchivedPostRequest) (*postpb.AllArchivedPostResponse, error) {
	posts, err := h.postService.GetAllArchivedPost(ctx, input.AuthorId)
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return &postpb.AllArchivedPostResponse{}, status.Error(codes.NotFound, "Post Not Found")
		}
		return nil, status.Errorf(codes.Internal, "failed to find post %v", err)
	}

	Posts := make([]*postpb.Post, 0, len(posts))

	for _, res := range posts {
		Posts = append(Posts, &postpb.Post{
			Id:        res.ID.String(),
			AuthorId:  res.AuthorID.String(),
			Content:   res.Content,
			MediaUrl:  res.MediaURL,
			IsPublic:  res.IsPublic,
			LikeCount: int32(res.LikeCount),
			CreatedAt: res.CreatedAt.Format(time.RFC3339),
			UpdatedAt: formatTimestamp(res.UpdatedAt),
		})
	}

	return &postpb.AllArchivedPostResponse{
		Post: Posts,
	}, nil
}

func (h *PostHandler) GetArchivedPostById(ctx context.Context, input *postpb.ArchivedPostByIdRequest) (*postpb.ArchivedPostByIdResponse, error) {
	post, err := h.postService.GetArchivedPostById(ctx, input.Id) 
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return &postpb.ArchivedPostByIdResponse{}, status.Error(codes.NotFound, "Post Not Found")
		}
		return nil, status.Errorf(codes.Internal, "failed to find post %v", err)
	}

	return &postpb.ArchivedPostByIdResponse{
		Post: &postpb.Post{
			Id:        post.ID.String(),
			AuthorId:  post.AuthorID.String(),
			Content:   post.Content,
			MediaUrl:  post.MediaURL,
			IsPublic:  post.IsPublic,
			LikeCount: int32(post.LikeCount),
			CreatedAt: post.CreatedAt.Format(time.RFC3339),
			UpdatedAt: formatTimestamp(post.UpdatedAt),
		},
	}, nil
}

func (h *PostHandler) MovePostToArchived(ctx context.Context, input *postpb.MovePostToArchivedRequest) (*postpb.MovePostToArchivedResponse, error) {
	_, err := h.postService.MovePostToArchive(ctx, input.PostId)
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return nil, status.Error(codes.NotFound, "Post Not Found")
		}

		return &postpb.MovePostToArchivedResponse{}, status.Errorf(codes.Internal, "Error Moving Post to Archive: %v", err)
	}

	return &postpb.MovePostToArchivedResponse{
		Message: "Post Moved to Archive Successfully",
	}, nil
}