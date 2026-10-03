package models

import "errors"

var (
	ErrUserAlreadyExists = errors.New("user already exists with this username or email")
	ErrUserNotFound      = errors.New("user not found")
	ErrInvalidInput      = errors.New("invalid input")
)
