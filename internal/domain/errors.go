package domain

import "errors"

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

