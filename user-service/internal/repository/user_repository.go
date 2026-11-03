package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/chetansachan/socialflow/user-service/internal/database"
	"github.com/chetansachan/socialflow/user-service/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepository interface {
	Create(ctx context.Context, user *models.User) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByUsername(ctx context.Context, username string) (*models.User, error)
	GetById(ctx context.Context, id uuid.UUID) (*models.User, error)
	GetByIds(ctx context.Context, ids []uuid.UUID) ([]models.User, error)
	SaveToken(ctx context.Context, data *models.RefreshTkn) error
	GetRefreshTk(ctx context.Context, tkn string) (*models.RefreshTkn, error)
	RotateToken(ctx context.Context, oldtkn string, newtkn string, data *models.RefreshTkn) error
	UpdateUser(ctx context.Context, input *models.UpdateUserInput, id uuid.UUID) (*models.User, error)
	DeleteUser(ctx context.Context, id uuid.UUID) error
}

type userRepository struct {
	writer *pgxpool.Pool
	reader *pgxpool.Pool
}

// NewUserRepository builds a repository over a primary pool and, optionally, a
// read replica. Auth-critical reads stay on the primary: a replication lag of
// even a second would make a freshly registered account look missing at login,
// or resurrect a refresh token that was just rotated.
func NewUserRepository(db *database.DB) UserRepository {
	return &userRepository{
		writer: db.Writer(),
		reader: db.Reader(),
	}
}

func (r *userRepository) Create(ctx context.Context, user *models.User) (*models.User, error) {
	query := `
		INSERT INTO users (username, name, email, password, is_public)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, username, name, email, password, is_public, created_at, updated_at
	`

	createdUser := &models.User{}
	err := r.writer.QueryRow(
		ctx,
		query,
		user.Username,
		user.Name,
		user.Email,
		user.Password,
		user.IsPublic,
	).Scan(
		&createdUser.ID,
		&createdUser.Username,
		&createdUser.Name,
		&createdUser.Email,
		&createdUser.Password,
		&createdUser.IsPublic,
		&createdUser.CreatedAt,
		&createdUser.UpdatedAt,
	)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, models.ErrUserAlreadyExists
		}
		return nil, fmt.Errorf("failed to insert user: %w", err)
	}

	return createdUser, nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, username, name, email, password, is_public, created_at, updated_at
		FROM users
		WHERE email = $1
	`

	user := &models.User{}
	err := r.writer.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Username,
		&user.Name,
		&user.Email,
		&user.Password,
		&user.IsPublic,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrUserNotFound
		}

		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}
	return user, nil
}

func (r *userRepository) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	query := `
		SELECT id, username, name, email, password, is_public, created_at, updated_at
		FROM users
		WHERE username = $1
	`

	user := &models.User{}
	err := r.reader.QueryRow(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Name,
		&user.Email,
		&user.Password,
		&user.IsPublic,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrUserNotFound
		}

		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}
	return user, nil
}

func (r *userRepository) GetById(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := `
		SELECT id, username, name, email, password, is_public, created_at, updated_at 
		FROM users 
		WHERE id = $1
	`

	user := &models.User{}
	if err := r.reader.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Username,
		&user.Name,
		&user.Email,
		&user.Password,
		&user.IsPublic,
		&user.CreatedAt,
		&user.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}

	return user, nil
}

// GetByIds performs one indexed lookup for a batch of ids. Missing ids are
// simply absent from the result, which lets dataloaders resolve partial pages.
func (r *userRepository) GetByIds(ctx context.Context, ids []uuid.UUID) ([]models.User, error) {
	if len(ids) == 0 {
		return []models.User{}, nil
	}

	query := `
		SELECT id, username, name, email, password, is_public, created_at, updated_at
		FROM users
		WHERE id = ANY($1)
	`

	rows, err := r.reader.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to batch get users: %w", err)
	}
	defer rows.Close()

	users := make([]models.User, 0, len(ids))
	for rows.Next() {
		var user models.User
		if err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Name,
			&user.Email,
			&user.Password,
			&user.IsPublic,
			&user.CreatedAt,
			&user.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan batch user: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating batch users: %w", err)
	}
	return users, nil
}

func (r *userRepository) SaveToken(ctx context.Context, data *models.RefreshTkn) error {
	query := `
		INSERT INTO refreshtkn (userid, token, expired_at)
		VALUES ($1, $2, $3)
	`

	_, err := r.writer.Exec(ctx, query, data.UserId, data.Token, data.ExpiresAt)
	if err != nil {
		return fmt.Errorf("Error saving token %w", err)
	}

	return nil
}

func (r *userRepository) GetRefreshTk(ctx context.Context, tkn string) (*models.RefreshTkn, error) {
	query := `
		SELECT id, userid, token, is_revoked, expired_at, created_at, updated_at 
		FROM refreshtkn 
		WHERE token = $1
	`

	Token := &models.RefreshTkn{}
	if err := r.writer.QueryRow(ctx, query, tkn).Scan(
		&Token.Id,
		&Token.UserId,
		&Token.Token,
		&Token.IsRevoked,
		&Token.ExpiresAt,
		&Token.CreatedAt,
		&Token.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("token not found %v", err)
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	return Token, nil
}

func (r *userRepository) RotateToken(ctx context.Context, oldtkn string, newtkn string, data *models.RefreshTkn) error {
	query := `	
		UPDATE refreshtkn
		SET token = $1, expired_at = $3, updated_at = $4
		WHERE token = $2;
	`

	res, err := r.writer.Exec(ctx, query, newtkn, oldtkn, data.ExpiresAt, data.UpdatedAt)
	if err != nil {
		return fmt.Errorf("Error saving token %w", err)
	}

	if res.RowsAffected() == 0 {
		return fmt.Errorf("refresh token not found or already rotated")
	}

	return nil
}

func (r *userRepository) UpdateUser(ctx context.Context, input *models.UpdateUserInput, id uuid.UUID) (*models.User, error) {
	query := `
		UPDATE users
		SET username = $1, name = $2, email = $3, password = $4, is_public = $5, updated_at = CURRENT_TIMESTAMP
		WHERE id = $6
		RETURNING id, username, name, email, password, is_public, created_at, updated_at
	`

	user := &models.User{}
	err := r.writer.QueryRow(ctx, query, input.Username, input.Name, input.Email, input.Password, input.IsPublic, id).Scan(
		&user.ID,
		&user.Username,
		&user.Name,
		&user.Email,
		&user.Password,
		&user.IsPublic,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrUserNotFound
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, models.ErrUserAlreadyExists
		}
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return user, nil
}

func (r *userRepository) DeleteUser(ctx context.Context, id uuid.UUID) error {
	query := `
		DELETE FROM users
		WHERE id = $1
	`

	_, err := r.writer.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("Error deleting User: %v", err)
	}

	return nil
}
