package model

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/eveenendaal/last-run/internal/db"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.InitDB(conn); err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestSelectMissing(t *testing.T) {
	task, err := Select(newTestDB(t), "nope")
	if err != nil || task != nil {
		t.Errorf("Select(missing) = (%+v, %v), want (nil, nil)", task, err)
	}
}

func TestEnsure(t *testing.T) {
	conn := newTestDB(t)

	task, created, err := Ensure(conn, "job")
	if err != nil || !created || task.ID != "job" {
		t.Fatalf("first Ensure = (%+v, %v, %v), want new task", task, created, err)
	}
	again, created, err := Ensure(conn, "job")
	if err != nil || created || again.ID != "job" {
		t.Errorf("second Ensure = (%+v, %v, %v), want existing task", again, created, err)
	}
}

func TestUpdateRoundTripAndLog(t *testing.T) {
	conn := newTestDB(t)
	task, _, err := Ensure(conn, "job")
	if err != nil {
		t.Fatal(err)
	}

	// Only a start time: no log entry yet.
	start := time.Date(2025, 3, 4, 5, 6, 7, 500_000_000, time.UTC)
	task.StartTime = &start
	if err := task.Update(conn); err != nil {
		t.Fatal(err)
	}
	if logs, _ := db.GetTaskLogs(conn, nil, 0); len(logs) != 0 {
		t.Fatalf("logs after start = %d, want 0", len(logs))
	}

	end := start.Add(2500 * time.Millisecond)
	task.LastRun = &end
	if err := task.Update(conn); err != nil {
		t.Fatal(err)
	}

	got, err := Select(conn, "job")
	if err != nil {
		t.Fatal(err)
	}
	if got.StartTime == nil || !got.StartTime.Equal(start) || got.LastRun == nil || !got.LastRun.Equal(end) {
		t.Errorf("round trip = %+v, want start %v / last run %v", got, start, end)
	}

	logs, err := db.GetTaskLogs(conn, new("job"), 0)
	if err != nil || len(logs) != 1 || logs[0].ElapsedMs != 2500 {
		t.Errorf("logs = (%+v, %v), want one entry of 2500ms", logs, err)
	}

	// Clearing both fields stores NULLs.
	task.StartTime, task.LastRun = nil, nil
	if err := task.Update(conn); err != nil {
		t.Fatal(err)
	}
	if got, _ := Select(conn, "job"); got.StartTime != nil || got.LastRun != nil {
		t.Errorf("after clear = %+v, want nil times", got)
	}
}
