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
	GetAllPost(ctx context.Context, authorId string) ([]models.Post, error)
	CreatePost(ctx context.Context, input models.CreatePostInput) (*models.Post, error)
	UpdatePost(ctx context.Context, input models.UpdatePostInput, postId string, authorId string) (*models.Post, error)
	DeletePost(ctx context.Context, postid string) (*models.Response, error)
	MovePostToArchive(ctx context.Context, postid string) (*models.Response, error)
	GetAllArchivedPost(ctx context.Context, authorId string) ([]models.Post, error)
	GetArchivedPostById(ctx context.Context, id string) (*models.Post, error)
	LikePost(ctx context.Context, postId string, userId string) (bool, error)
	// GetAllPostLikers(ctx context.Context, postId string) ([]string, error)
	// GetAllLikedPosts(ctx context.Context, userId string) ([]models.Post, error)
}

type postService struct {
	postRepo repository.PostRepository
}

func NewPostService(postRepo repository.PostRepository) PostService {
	return &postService{postRepo: postRepo}
}

func (s *postService) GetPostById(ctx context.Context, id string) (*models.Post, error) {
	postId, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse the id: %v", err)
	}

	post, err := s.postRepo.GetPostById(ctx, postId)
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return nil, models.ErrPostNotFound
		}
		return nil, fmt.Errorf("Internal Server Error: %w", err)
	}

	return post, nil
}

func (s *postService) GetAllPost(ctx context.Context, authorId string) ([]models.Post, error) {
	authorid, err := uuid.Parse(authorId)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse the id: %v", err)
	}

	posts, err := s.postRepo.GetAllPost(ctx, authorid)
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return nil, models.ErrPostNotFound
		}
		return nil, fmt.Errorf("Internal Server Error: %w", err)
	}

	return posts, nil
}

func (s *postService) CreatePost(ctx context.Context, input models.CreatePostInput) (*models.Post, error) {
	if input.AuthorID == uuid.Nil {
		return nil, models.ErrInvalidInput
	}

	if input.Content == "" {
		return nil, models.ErrInvalidInput
	}

	if input.MediaURL == "" {
		return nil, models.ErrInvalidInput
	}

	isPublic := true
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	}

	postinput := &models.CreatePostInput{
		AuthorID: input.AuthorID,
		Content:  input.Content,
		MediaURL: input.MediaURL,
		IsPublic: &isPublic,
	}
	post, err := s.postRepo.CreatePost(ctx, postinput)
	if err != nil {
		return nil, fmt.Errorf("Error Creating Post: %v", err)
	}

	return post, nil
}

func (s *postService) UpdatePost(ctx context.Context, input models.UpdatePostInput, postid string, authorid string) (*models.Post, error) {
	postId, err := uuid.Parse(postid)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse the id: %v", err)
	}

	authorId, err := uuid.Parse(authorid)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse the id: %v", err)
	}

	post, err := s.postRepo.GetPostById(ctx, postId)
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return nil, fmt.Errorf("Post Not Found: %w", err)
		}
		return nil, fmt.Errorf("Internal Server Error: %w", err)
	}

	content := post.Content
	if input.Content != nil {
		content = *input.Content
	}

	isPublic := post.IsPublic
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	}

	updateinput := models.UpdatePostInput{
		Content:  &content,
		IsPublic: &isPublic,
	}

	updatedPost, err := s.postRepo.UpdatePost(ctx, &updateinput, postId, authorId)
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return nil, models.ErrPostNotFound
		}
		return nil, fmt.Errorf("Internal Server Error: %v", err)
	}

	return updatedPost, nil
}

func (s *postService) DeletePost(ctx context.Context, postid string) (*models.Response, error) {
	postId, err := uuid.Parse(postid)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse the id: %v", err)
	}

	if err := s.postRepo.DeletePost(ctx, postId); err != nil {
		return nil, fmt.Errorf("Error Deleting Post: %v", err)
	}

	return &models.Response{
		Message: "Post Deleted Successfully",
	}, nil
}

func (s *postService) MovePostToArchive(ctx context.Context, postid string) (*models.Response, error) {
	postId, err := uuid.Parse(postid)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse the id: %v", err)
	}

	if err := s.postRepo.MovePostToArchive(ctx, postId); err != nil {
		return nil, fmt.Errorf("Error Moving Post to archive: %v", err)
	}

	return &models.Response{
		Message: "Post Moved to archive Successfully",
	}, nil
}

func (s *postService) GetAllArchivedPost(ctx context.Context, authorId string) ([]models.Post, error) {
	authorid, err := uuid.Parse(authorId)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse the id: %v", err)
	}

	posts, err := s.postRepo.GetAllArchivedPost(ctx, authorid)
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return nil, models.ErrPostNotFound
		}
		return nil, fmt.Errorf("Internal Server Error: %w", err)
	}

	return posts, nil
}

func (s *postService) GetArchivedPostById(ctx context.Context, id string) (*models.Post, error) {
	postId, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse the id: %v", err)
	}

	post, err := s.postRepo.GetArchivedPostById(ctx, postId)
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return nil, models.ErrPostNotFound
		}
		return nil, fmt.Errorf("Internal Server Error: %w", err)
	}

	return post, nil
}

func (s *postService) LikePost(ctx context.Context, postId string, userId string) (bool, error) {
	postUUID, err := uuid.Parse(postId)
	if err != nil {
		return false, fmt.Errorf("unable to parse the post id: %w", err)
	}

	userUUID, err := uuid.Parse(userId)
	if err != nil {
		return false, fmt.Errorf("unable to parse the user id: %w", err)
	}

	isLiked, err := s.postRepo.LikePost(ctx, postUUID, userUUID)
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return false, models.ErrPostNotFound
		}
		return false, fmt.Errorf("failed to like post: %w", err)
	}

	return isLiked, nil
}

// func (s *postService) GetAllPostLikers(ctx context.Context, postId string) ([]string, error) {
// 	postUUID, err := uuid.Parse(postId)
// 	if err != nil {
// 		return nil, fmt.Errorf("unable to parse the post id: %w", err)
// 	}

// 	likers, err := s.postRepo.GetAllPostLikers(ctx, postUUID)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get post likers: %w", err)
// 	}

// 	return likers, nil
// }

// func (s *postService) GetAllLikedPosts(ctx context.Context, userId string) ([]models.Post, error) {
// 	userUUID, err := uuid.Parse(userId)
// 	if err != nil {
// 		return nil, fmt.Errorf("unable to parse the user id: %w", err)
// 	}

// 	posts, err := s.postRepo.GetAllLikedPosts(ctx, userUUID)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get liked posts: %w", err)
// 	}

// 	return posts, nil
// }

