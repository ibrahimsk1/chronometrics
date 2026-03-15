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

// Sentinel validation errors — one per validated field.
var (
	ErrRawEventNil          = errors.New("raw event is nil")
	ErrEventNameMissing     = errors.New("event_name is required")
	ErrUserIDMissing        = errors.New("user_id is required")
	ErrTimestampNonPositive = errors.New("timestamp must be positive")
	ErrTimestampTooFuture   = errors.New("timestamp too far in future")
	ErrTimestampTooPast     = errors.New("timestamp too far in past")

	ErrQueryEventNameMissing = errors.New("query event_name is required")
	ErrQueryFromMissing      = errors.New("query from is required")
	ErrQueryToMissing        = errors.New("query to is required")
	ErrQueryRangeInvalid     = errors.New("query from must be before to")
	ErrQueryGroupByInvalid   = errors.New("query group_by is invalid")
)

// Helper predicates for sentinel errors.
func IsPublishFailed(err error) bool {
	return errors.Is(err, ErrPublishFailed)
}

func IsQueryTimeout(err error) bool {
	return errors.Is(err, ErrQueryTimeout)
}

// ValidationError indicates input failed domain validation.
// Unwrap returns all collected errors so errors.Is works against sentinels.
type ValidationError struct {
	Errors []error
}

func (v *ValidationError) Error() string {
	if len(v.Errors) == 0 {
		return "validation failed"
	}
	msgs := make([]string, len(v.Errors))
	for i, e := range v.Errors {
		msgs[i] = e.Error()
	}
	return "validation failed: " + strings.Join(msgs, "; ")
}

// Unwrap exposes all wrapped errors so errors.Is can traverse them.
func (v *ValidationError) Unwrap() []error {
	return v.Errors
}

// IsValidationError returns true when err is a ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
