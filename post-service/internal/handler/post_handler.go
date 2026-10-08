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

type postHandler struct {
	postpb.UnimplementedPostServiceServer
	postService services.PostService
}

func NewPostHandler(postService services.PostService) postHandler {
	return postHandler{postService: postService}
}

func formatTimestamp(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func (h *postHandler) CreatePostHandler(ctx context.Context, req *postpb.CreatePostRequest) (*postpb.CreatePostResponse, error) {
	authorId, err := uuid.Parse(req.AuthorId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "invalid author ID format: %v", err)
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
			return nil, models.ErrInvalidInput
		}

		return nil, status.Errorf(codes.Internal, "faield to create post: %v", err)
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

func (h *postHandler) GetPostByIdHandler(ctx context.Context, input *postpb.PostByIdRequest) (*postpb.PostByIdResponse, error) {
	res, err := h.postService.GetPostById(ctx, input.Id)
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return &postpb.PostByIdResponse{}, models.ErrPostNotFound
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

func (h *postHandler) DeletePostHandler(ctx context.Context, input *postpb.DeletePostRequest) (*postpb.DeletePostResponse, error) {
	_, err := h.postService.DeletePost(ctx, input.PostId, input.AuthorId)
	if err != nil {
		return &postpb.DeletePostResponse{}, status.Errorf(codes.Internal, "Error Deleting Post: %v", err)
	}

	return &postpb.DeletePostResponse{
		Message: "Post Deleted Successfully",
	}, nil
}
