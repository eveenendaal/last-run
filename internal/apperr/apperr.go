// Package apperr defines the application's sentinel errors and error types,
// mirroring the variants of the original Rust AppError enum.
package apperr

import "errors"

// Sentinel errors for conditions that carry no extra data.
var (
	// ErrMissingTaskID indicates a required task ID was empty.
	ErrMissingTaskID = errors.New("Task ID is required")
	// ErrDataDirectoryNotFound indicates the platform data directory could
	// not be resolved.
	ErrDataDirectoryNotFound = errors.New("Data directory not found")
)

// DurationParseError represents an invalid duration string. It mirrors the
// Rust AppError::DurationParse(String) variant.
type DurationParseError struct {
	Msg string
}

func (e *DurationParseError) Error() string {
	return "Duration parsing error: " + e.Msg
}

// NewDurationParseError builds a DurationParseError from a message.
func NewDurationParseError(msg string) error {
	return &DurationParseError{Msg: msg}
}
