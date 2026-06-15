package cli

import (
	"strings"
	"testing"
	"time"
)

func TestShouldRunTask(t *testing.T) {
	now := time.Now().UTC()

	// Ran 10h ago, threshold 24h -> not due.
	shouldRun, msg := ShouldRunTask(now.Add(-10*time.Hour), 24*time.Hour)
	if shouldRun {
		t.Error("expected not due for 10h ago / 24h threshold")
	}
	if !strings.Contains(msg, "not due yet") || !strings.Contains(msg, "10h") {
		t.Errorf("unexpected message: %q", msg)
	}

	// Ran 30h ago, threshold 24h -> due.
	shouldRun, msg = ShouldRunTask(now.Add(-30*time.Hour), 24*time.Hour)
	if !shouldRun {
		t.Error("expected due for 30h ago / 24h threshold")
	}
	if !strings.Contains(msg, "Task is due") || !strings.Contains(msg, "1d6h") {
		t.Errorf("unexpected message: %q", msg)
	}

	// Ran exactly at threshold -> due.
	shouldRun, msg = ShouldRunTask(now.Add(-24*time.Hour), 24*time.Hour)
	if !shouldRun {
		t.Error("expected due at exactly threshold")
	}
	if !strings.Contains(msg, "Task is due") {
		t.Errorf("unexpected message: %q", msg)
	}

	// Ran 6 days ago, threshold 7 days -> not due.
	shouldRun, msg = ShouldRunTask(now.Add(-6*24*time.Hour), 7*24*time.Hour)
	if shouldRun {
		t.Error("expected not due for 6d ago / 7d threshold")
	}
	if !strings.Contains(msg, "not due yet") || !strings.Contains(msg, "6d") {
		t.Errorf("unexpected message: %q", msg)
	}

	// Ran 8 days ago, threshold 7 days -> due.
	shouldRun, msg = ShouldRunTask(now.Add(-8*24*time.Hour), 7*24*time.Hour)
	if !shouldRun {
		t.Error("expected due for 8d ago / 7d threshold")
	}
	if !strings.Contains(msg, "Task is due") || !strings.Contains(msg, "8d") {
		t.Errorf("unexpected message: %q", msg)
	}
}
