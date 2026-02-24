package handler

import "errors"

// ErrQueryTimeout indicates the metrics backend timed out.
var ErrQueryTimeout = errors.New("query timeout")
