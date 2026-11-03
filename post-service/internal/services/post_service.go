package services

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/chetansachan/socialflow/post-service/internal/config"
	"github.com/chetansachan/socialflow/post-service/internal/models"
	"github.com/chetansachan/socialflow/post-service/internal/repository"
	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"github.com/google/uuid"
)

type PostService interface {
	GetPostById(ctx context.Context, id string) (*models.Post, error)
	GetAllPost(ctx context.Context, authorId string) ([]models.Post, error)
	CreatePost(ctx context.Context, input models.CreatePostInput) (*models.Post, error)
	UpdatePost(ctx context.Context, input models.UpdatePostInput, postId string, authorId string) (*models.Post, error)
	DeletePost(ctx context.Context, postId string, authorId string) (*models.Response, error)
	MovePostToArchive(ctx context.Context, postId string, authorId string) (*models.Response, error)
	GetAllArchivedPost(ctx context.Context, authorId string) ([]models.Post, error)
	GetArchivedPostById(ctx context.Context, id string) (*models.Post, error)
	LikePost(ctx context.Context, postId string, userId string) (bool, error)
	GetAllPostLikers(ctx context.Context, postId string) ([]string, error)
	GetAllLikedPosts(ctx context.Context, userId string) ([]models.Post, error)
	GetLikeStates(ctx context.Context, userId string, postIds []string) (map[string]bool, error)
	Upload(ctx context.Context, base64 string) (string, error)
	CreateUploadSignature(ctx context.Context, userId, resourceType, folder string) (*models.UploadSignature, error)
}

// maxPostLength bounds the text of a single post. Media travels separately, so
// this only limits the caption.
const maxPostLength = 1000

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

	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, models.ErrInvalidInput
	}
	if utf8.RuneCountInString(content) > maxPostLength {
		return nil, fmt.Errorf("%w: post content must be at most %d characters", models.ErrInvalidInput, maxPostLength)
	}

	mediaURL := input.MediaURL
	if mediaURL != "" {
		uploadedURL, err := s.Upload(ctx, mediaURL)
		if err != nil {
			return nil, fmt.Errorf("failed to upload media: %w", err)
		}
		mediaURL = uploadedURL
	}

	isPublic := true
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	}

	postinput := &models.CreatePostInput{
		AuthorID: input.AuthorID,
		Content:  content,
		MediaURL: mediaURL,
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
		return nil, fmt.Errorf("%w: invalid post id", models.ErrInvalidInput)
	}

	authorId, err := uuid.Parse(authorid)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid author id", models.ErrInvalidInput)
	}

	post, err := s.requirePostOwner(ctx, postId, authorId)
	if err != nil {
		return nil, err
	}

	content := post.Content
	if input.Content != nil {
		content = strings.TrimSpace(*input.Content)
		if utf8.RuneCountInString(content) > maxPostLength {
			return nil, fmt.Errorf("%w: post content must be at most %d characters", models.ErrInvalidInput, maxPostLength)
		}
		if content == "" && strings.TrimSpace(post.MediaURL) == "" {
			return nil, fmt.Errorf("%w: post must keep text or media", models.ErrInvalidInput)
		}
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

func (s *postService) DeletePost(ctx context.Context, postid string, authorid string) (*models.Response, error) {
	postId, err := uuid.Parse(postid)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid post id", models.ErrInvalidInput)
	}

	authorId, err := uuid.Parse(authorid)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid author id", models.ErrInvalidInput)
	}

	if _, err := s.requirePostOwner(ctx, postId, authorId); err != nil {
		return nil, err
	}

	if err := s.postRepo.DeletePost(ctx, postId, authorId); err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return nil, models.ErrPostNotFound
		}
		return nil, fmt.Errorf("Error Deleting Post: %v", err)
	}

	return &models.Response{
		Message: "Post Deleted Successfully",
	}, nil
}

func (s *postService) MovePostToArchive(ctx context.Context, postid string, authorid string) (*models.Response, error) {
	postId, err := uuid.Parse(postid)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid post id", models.ErrInvalidInput)
	}

	authorId, err := uuid.Parse(authorid)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid author id", models.ErrInvalidInput)
	}

	if _, err := s.requirePostOwner(ctx, postId, authorId); err != nil {
		return nil, err
	}

	if err := s.postRepo.MovePostToArchive(ctx, postId, authorId); err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return nil, models.ErrPostNotFound
		}
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

func (s *postService) GetAllPostLikers(ctx context.Context, postId string) ([]string, error) {
	postUUID, err := uuid.Parse(postId)
	if err != nil {
		return nil, fmt.Errorf("unable to parse the post id: %w", err)
	}

	likers, err := s.postRepo.GetAllPostLikers(ctx, postUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get post likers: %w", err)
	}

	return likers, nil
}

func (s *postService) GetAllLikedPosts(ctx context.Context, userId string) ([]models.Post, error) {
	userUUID, err := uuid.Parse(userId)
	if err != nil {
		return nil, fmt.Errorf("unable to parse the user id: %w", err)
	}

	posts, err := s.postRepo.GetAllLikedPosts(ctx, userUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get liked posts: %w", err)
	}

	return posts, nil
}

// GetLikeStates resolves the viewer's like state for a batch of posts, which
// lets the API gateway render feed pages with one RPC instead of N.
func (s *postService) GetLikeStates(ctx context.Context, userId string, postIds []string) (map[string]bool, error) {
	userUUID, err := uuid.Parse(userId)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid user id", models.ErrInvalidInput)
	}

	parsed := make([]uuid.UUID, 0, len(postIds))
	seen := make(map[uuid.UUID]struct{}, len(postIds))
	for _, raw := range postIds {
		id, err := uuid.Parse(raw)
		if err != nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		parsed = append(parsed, id)
	}

	return s.postRepo.GetLikeStates(ctx, userUUID, parsed)
}

// CreateUploadSignature signs a direct-to-Cloudinary upload. The secret never
// leaves the service; the client only receives the short-lived signature.
func (s *postService) CreateUploadSignature(ctx context.Context, userId, resourceType, folder string) (*models.UploadSignature, error) {
	userUUID, err := uuid.Parse(userId)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid user id", models.ErrInvalidInput)
	}

	cfg := config.Load()
	parsed, err := url.Parse(strings.TrimSpace(cfg.CloudinaryUrl))
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("cloudinary is not configured")
	}
	apiKey := parsed.User.Username()
	apiSecret, hasSecret := parsed.User.Password()
	if apiKey == "" || !hasSecret || apiSecret == "" {
		return nil, fmt.Errorf("cloudinary credentials are incomplete")
	}

	resource := strings.ToLower(strings.TrimSpace(resourceType))
	switch resource {
	case "", "auto":
		resource = "auto"
	case "image", "video", "raw":
	default:
		return nil, fmt.Errorf("%w: unsupported resource type", models.ErrInvalidInput)
	}

	folder = strings.Trim(strings.TrimSpace(folder), "/")
	if folder == "" {
		folder = "socialflow/" + userUUID.String()
	}

	timestamp := time.Now().Unix()
	publicID := uuid.NewString()
	signature := sha1.Sum([]byte(fmt.Sprintf(
		"folder=%s&public_id=%s&timestamp=%d%s",
		folder, publicID, timestamp, apiSecret,
	)))

	return &models.UploadSignature{
		CloudName: parsed.Host,
		APIKey:    apiKey,
		Timestamp: timestamp,
		Signature: hex.EncodeToString(signature[:]),
		Folder:    folder,
		PublicID:  publicID,
		UploadURL: fmt.Sprintf("https://api.cloudinary.com/v1_1/%s/%s/upload", parsed.Host, resource),
		ExpiresAt: timestamp + 600,
	}, nil
}

// requirePostOwner loads the post and confirms the caller owns it. The check
// lives in the service so every transport (GraphQL today, anything else
// tomorrow) gets the same rule.
func (s *postService) requirePostOwner(ctx context.Context, postID, authorID uuid.UUID) (*models.Post, error) {
	post, err := s.postRepo.GetPostById(ctx, postID)
	if err != nil {
		if errors.Is(err, models.ErrPostNotFound) {
			return nil, models.ErrPostNotFound
		}
		return nil, fmt.Errorf("Internal Server Error: %w", err)
	}
	if post.AuthorID != authorID {
		return nil, models.ErrUnauthorized
	}
	return post, nil
}

func inferMimeType(ext string) string {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	switch ext {
	case "mp4", "mov", "webm", "avi", "mkv", "ogv", "m4v":
		return "video/" + ext
	case "mp3", "wav", "ogg", "m4a", "aac":
		return "audio/" + ext
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "svg":
		return "image/svg+xml"
	default:
		return "image/" + ext
	}
}

func (s *postService) Upload(ctx context.Context, base64 string) (string, error) {
	mediaData := strings.TrimSpace(base64)
	if mediaData == "" {
		return "", fmt.Errorf("media data is empty")
	}

	var mimeType string
	var ext string
	payload := mediaData

	if strings.HasPrefix(mediaData, "data:") {
		commaIdx := strings.Index(mediaData, ",")
		if commaIdx != -1 {
			header := mediaData[len("data:"):commaIdx]
			parts := strings.Split(header, ";")
			if len(parts) > 0 {
				mimeType = strings.TrimSpace(parts[0])
			}
		}

		if mimeType != "" {
			parts := strings.SplitN(mimeType, "/", 2)
			if len(parts) == 2 {
				ext = parts[1]
				if plusIdx := strings.Index(ext, "+"); plusIdx != -1 {
					ext = ext[:plusIdx]
				}
			}
		}
		payload = mediaData
	} else if strings.HasPrefix(mediaData, "http://") || strings.HasPrefix(mediaData, "https://") {
		if strings.Contains(mediaData, "res.cloudinary.com") {
			ext = strings.TrimPrefix(path.Ext(mediaData), ".")
			if ext == "" {
				ext = "jpg"
			}
			mimeType = inferMimeType(ext)
			_ = s.postRepo.SaveFiles(ctx, mediaData, ext, mimeType)
			return mediaData, nil
		}
		ext = strings.TrimPrefix(path.Ext(mediaData), ".")
		if ext == "" {
			ext = "jpg"
		}
		mimeType = inferMimeType(ext)
		payload = mediaData
	} else {
		// Raw base64 data without data: URI header
		if strings.HasPrefix(mediaData, "/9j/") {
			mimeType = "image/jpeg"
			ext = "jpeg"
		} else if strings.HasPrefix(mediaData, "iVBORw") {
			mimeType = "image/png"
			ext = "png"
		} else if strings.HasPrefix(mediaData, "R0lGOD") {
			mimeType = "image/gif"
			ext = "gif"
		} else if strings.HasPrefix(mediaData, "UklGR") {
			mimeType = "image/webp"
			ext = "webp"
		} else if strings.HasPrefix(mediaData, "AAAA") {
			mimeType = "video/mp4"
			ext = "mp4"
		} else {
			mimeType = "application/octet-stream"
			ext = "bin"
		}
		payload = fmt.Sprintf("data:%s;base64,%s", mimeType, mediaData)
	}

	url, err := s.UploadFile(ctx, payload)
	if err != nil {
		return "", fmt.Errorf("error uploading file: %w", err)
	}

	if ext == "" {
		ext = strings.TrimPrefix(path.Ext(url), ".")
		if ext == "" {
			ext = "bin"
		}
	}
	if mimeType == "" {
		mimeType = inferMimeType(ext)
	}

	if err := s.postRepo.SaveFiles(ctx, url, ext, mimeType); err != nil {
		return "", fmt.Errorf("error saving file record: %w", err)
	}

	return url, nil
}

func (s *postService) UploadFile(ctx context.Context, payload string) (string, error) {
	cfg := config.Load()
	if cfg.CloudinaryUrl == "" {
		return "", fmt.Errorf("cloudinary URL is not configured")
	}

	cld, err := cloudinary.NewFromURL(cfg.CloudinaryUrl)
	if err != nil {
		return "", fmt.Errorf("error initializing cloudinary: %w", err)
	}

	resp, err := cld.Upload.Upload(ctx, payload, uploader.UploadParams{
		ResourceType:   "auto",
		UniqueFilename: api.Bool(true),
		Overwrite:      api.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}

	if resp.SecureURL != "" {
		return resp.SecureURL, nil
	}
	return resp.URL, nil
}
