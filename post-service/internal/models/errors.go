package models

import "errors"

var (
	ErrPostNotFound = errors.New("post not found")
	ErrInvalidInput = errors.New("invalid input")
	// ErrUnauthorized is returned when the caller is not the post author.
	ErrUnauthorized = errors.New("not authorized to modify this post")
)
