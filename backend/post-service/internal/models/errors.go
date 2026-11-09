package models

import "errors"

var (
	ErrPostNotFound = errors.New("post not found")
	ErrInvalidInput = errors.New("invalid input")
	// ErrUnauthorized is returned when the caller is not the post author.
	ErrUnauthorized = errors.New("not authorized to modify this post")
	// ErrCommentNotFound is returned when a comment does not exist or was
	// hard-deleted with its post.
	ErrCommentNotFound = errors.New("comment not found")
	// ErrCommentForbidden is returned when the caller is not the comment
	// author. It is distinct from ErrUnauthorized so handlers can return a
	// message that names the comment.
	ErrCommentForbidden = errors.New("not authorized to modify this comment")
	// ErrCommentDepthExceeded is returned when a reply would nest deeper than
	// the thread limit, keeping threads renderable in one pass.
	ErrCommentDepthExceeded = errors.New("comment thread depth exceeded")
)
