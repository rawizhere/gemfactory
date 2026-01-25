package model

import "errors"

var (
	ErrInternal = errors.New("internal server error")

	ErrNotFound = errors.New("resource not found")

	ErrForbidden = errors.New("access forbidden")

	ErrInvalidInput = errors.New("invalid input")

	ErrUnauthorized = errors.New("unauthorized")

	ErrRateLimit = errors.New("rate limit exceeded")
)
