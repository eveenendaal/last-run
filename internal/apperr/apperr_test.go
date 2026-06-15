package apperr

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestErrorMessages(t *testing.T) {
	if got := ErrMissingTaskID.Error(); got != "Task ID is required" {
		t.Errorf("ErrMissingTaskID = %q, want %q", got, "Task ID is required")
	}
	if got := ErrDataDirectoryNotFound.Error(); got != "Data directory not found" {
		t.Errorf("ErrDataDirectoryNotFound = %q, want %q", got, "Data directory not found")
	}

	durErr := NewDurationParseError("Invalid duration format")
	if !strings.Contains(durErr.Error(), "Duration parsing error") {
		t.Errorf("DurationParseError = %q, want to contain 'Duration parsing error'", durErr.Error())
	}
}

func TestErrorWrapping(t *testing.T) {
	// Sentinel errors are matchable through wrapping with errors.Is.
	wrapped := fmt.Errorf("context: %w", ErrMissingTaskID)
	if !errors.Is(wrapped, ErrMissingTaskID) {
		t.Error("errors.Is did not match wrapped ErrMissingTaskID")
	}

	// DurationParseError is extractable with errors.As.
	var dpe *DurationParseError
	err := fmt.Errorf("wrap: %w", NewDurationParseError("bad"))
	if !errors.As(err, &dpe) {
		t.Fatal("errors.As did not extract *DurationParseError")
	}
	if dpe.Msg != "bad" {
		t.Errorf("DurationParseError.Msg = %q, want %q", dpe.Msg, "bad")
	}
}
