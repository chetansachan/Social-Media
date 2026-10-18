package models

import "errors"

var (
	ErrCannotFollowSelf = errors.New("cannot follow yourself")
	ErrAlreadyFollowing = errors.New("already following this user")
	ErrRelationNotFound = errors.New("follow relationship not found")
	ErrInvalidID        = errors.New("invalid user id format")
)
