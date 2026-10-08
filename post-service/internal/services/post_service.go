package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/chetansachan/socialflow/post-service/internal/models"
	"github.com/chetansachan/socialflow/post-service/internal/repository"
	"github.com/google/uuid"
)

type PostService interface {
	GetPostById(ctx context.Context, id string) (*models.Post, error)
	CreatePost(ctx context.Context, input models.CreatePostInput) (*models.Post, error)
}

type postService struct {
	postRepo repository.PostRepostiory
}

func NewPostRepository(postRepo repository.PostRepostiory) PostService {
	return &postService{postRepo: postRepo}
}

func (s *postService) GetPostById(ctx context.Context, id string) (*models.Post, error) {
	postId, err := uuid.Parse(id)
	if err != nil {
		return &models.Post{}, fmt.Errorf("Unable to parse the id: %v", err)
	}

	post, err := s.postRepo.GetPostById(ctx, postId.String())
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound){
			return &models.Post{}, fmt.Errorf("Post Not Found: %v", err)
		}
		return &models.Post{}, fmt.Errorf("Internal Server Error: %v", err)
	}

	return post, nil
}

func (s *postService) CreatePost(ctx context.Context, input models.CreatePostInput) (*models.Post, error) {
	postinput := &models.Post{
		AuthorID: input.AuthorID,
		Content: input.Content,
		MediaURL: input.MediaURL,
		IsPublic: input.IsPublic,
	}
	post, err := s.postRepo.CreatePost(ctx, postinput)
	if err != nil {
		return &models.Post{}, fmt.Errorf("Error Creating Post: %v", err)
	}

	return post, nil
}
