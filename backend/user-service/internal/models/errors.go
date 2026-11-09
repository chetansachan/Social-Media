package models

import "errors"

var (
	ErrUserAlreadyExists = errors.New("user already exists with this username or email")
	ErrUserNotFound      = errors.New("user not found")
	ErrInvalidInput      = errors.New("invalid input")
	// ErrInvalidCredentials is deliberately identical for an unknown email and
	// a wrong password so login cannot be used to enumerate accounts.
	ErrInvalidCredentials = errors.New("invalid email or password")
)
