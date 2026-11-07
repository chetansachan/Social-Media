package models

import "errors"

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrUserNotFound = errors.New("user not found")
	// ErrNotParticipant is returned when a caller reads or writes a
	// conversation they are not a member of. It is deliberately generic: it
	// must not reveal whether the conversation exists.
	ErrNotParticipant       = errors.New("not a participant in this conversation")
	ErrConversationNotFound = errors.New("conversation not found")
	ErrMessageNotFound      = errors.New("message not found")
	// ErrCannotModerateSelf stops a user from opening a thread with themselves.
	ErrCannotModerateSelf = errors.New("cannot start a conversation with yourself")
)
