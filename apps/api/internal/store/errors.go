package store

import "errors"

// ErrNotFound is returned when a queried row does not exist. Service code
// maps it to the appropriate ApplicationError per use case.
var ErrNotFound = errors.New("record not found")
