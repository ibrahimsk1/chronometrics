package domain

import (
	"errors"
	"strings"
)

// Sentinel errors for domain operations.
var (
	ErrPublishFailed = errors.New("publish failed")
	ErrQueryTimeout  = errors.New("query timeout")
)

// Helper predicates for sentinel errors.
func IsPublishFailed(err error) bool {
	return errors.Is(err, ErrPublishFailed)
}

func IsQueryTimeout(err error) bool {
	return errors.Is(err, ErrQueryTimeout)
}

// ValidationError indicates input failed domain validation.
type ValidationError struct {
	Errors []string
}

func (v *ValidationError) Error() string {
	// join messages for caller-friendly error string
	if len(v.Errors) == 0 {
		return "validation failed"
	}
	return "validation failed: " + strings.Join(v.Errors, "; ")
}

// IsValidationError returns true when err is a ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
