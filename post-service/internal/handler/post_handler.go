package handler

import (
	"context"
	"errors"
	"time"

	"github.com/chetansachan/socialflow/post-service/internal/models"
	"github.com/chetansachan/socialflow/post-service/internal/services"
	postpb "github.com/chetansachan/socialflow/proto"
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


// func (h *postHandler) DeletePostHandler(ctx context.Context, input *postpb.DeletePostRequest) (*postpb.DeletePostResponse, error) {
// 	_, err := h.postService.DeletePost(ctx, input.PostId, input.AuthorId)
// 	if err != nil {
// 		return &postpb.DeletePostResponse{}, status.Errorf(codes.Internal, "Error Deleting Post: %v", err)
// 	}

// 	return &postpb.DeletePostResponse{
// 		Message: "Post Deleted Successfully",
// 	}, nil
// }