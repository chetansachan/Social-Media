package models

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	IsPublic  bool      `json:"is_public"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateUserInput struct {
	Username string `json:"username,omitempty"`
	Name     string `json:"name,omitempty"`
	Email    string `json:"email"`
	Password string `json:"password"`
	IsPublic *bool  `json:"is_public,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessTkn  string `json:"access_tkn"`
	RefreshTkn string `json:"refresh_tkn"`
	Message    string `json:"message"`
}

type UpdateUserInput struct {
	Username *string `json:"username,omitempty"`
	Name     *string `json:"name,omitempty"`
	Email    *string `json:"email,omitempty"`
	Password *string `json:"password,omitempty"`
	IsPublic *bool   `json:"is_public,omitempty"`
}

type MyCustomClaims struct {
	UserId string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}
