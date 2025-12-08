package domain

import "errors"

// Domain-level errors
var (
	ErrNotFound         = errors.New("resource not found")
	ErrAlreadyExists    = errors.New("resource already exists")
	ErrInvalidInput     = errors.New("invalid input")
	ErrInternalError    = errors.New("internal error")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrForbidden        = errors.New("forbidden")
	ErrValidationFailed = errors.New("validation failed")
	ErrDatabaseError    = errors.New("database error")
	ErrQueueError       = errors.New("queue error")
	ErrParsingError     = errors.New("parsing error")
	ErrGPUNotFound      = errors.New("GPU not found")
)
