package display

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eveenendaal/last-run/internal/db"
)

func TestWriteTaskStatusJSON(t *testing.T) {
	now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { return new(now.Add(-d)) }
	hour := int64(3600)

	tasks := []db.TaskStatus{
		{ID: "never"},
		{ID: "running", StartTime: at(90 * time.Second)},
		{ID: "overdue", StartTime: at(3 * time.Hour), LastRun: at(2 * time.Hour), Duration: &hour},
	}

	var buf bytes.Buffer
	if err := WriteTaskStatusJSON(&buf, tasks, now); err != nil {
		t.Fatal(err)
	}

	var got statusJSON
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if got.Timestamp != "2025-01-02T12:00:00+00:00" {
		t.Errorf("timestamp = %q", got.Timestamp)
	}
	if len(got.Tasks) != 3 {
		t.Fatalf("len(tasks) = %d, want 3", len(got.Tasks))
	}

	never, running, overdue := got.Tasks[0], got.Tasks[1], got.Tasks[2]

	if never.Status != "unknown" || never.LastRun != nil || never.ElapsedTime != nil || never.Duration != nil {
		t.Errorf("never = %+v", never)
	}

	if running.Status != "running" || running.StartTime == nil || running.LastRun != nil {
		t.Errorf("running = %+v", running)
	}
	if running.ElapsedTime == nil || *running.ElapsedTime != 90_000 || *running.ElapsedTimeFormatted != "1m30s" {
		t.Errorf("running elapsed = %v / %v, want 90000 / 1m30s", running.ElapsedTime, running.ElapsedTimeFormatted)
	}

	if overdue.Status != "due" {
		t.Errorf("overdue status = %q, want due", overdue.Status)
	}
	if overdue.LastRun == nil || *overdue.LastRun != "2025-01-02T10:00:00+00:00" {
		t.Errorf("overdue last_run = %v", overdue.LastRun)
	}
	if *overdue.TimeSinceLastRun != 7_200_000 || *overdue.TimeSinceLastRunFormatted != "2h0m" {
		t.Errorf("overdue since = %d / %s", *overdue.TimeSinceLastRun, *overdue.TimeSinceLastRunFormatted)
	}
	if *overdue.ElapsedTime != 3_600_000 || *overdue.Duration != 3600 || *overdue.DurationFormatted != "1h0m" {
		t.Errorf("overdue = %+v", overdue)
	}

	// Nil fields are emitted as explicit nulls so consumers see every key.
	if !strings.Contains(buf.String(), `"last_run": null`) {
		t.Errorf("expected explicit null fields:\n%s", buf.String())
	}
}

func TestWriteTaskStatusJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteTaskStatusJSON(&buf, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"tasks": []`) {
		t.Errorf("empty status should have an empty tasks array:\n%s", buf.String())
	}
}

func TestWriteTaskLogs(t *testing.T) {
	var buf bytes.Buffer
	WriteTaskLogs(&buf, nil)
	if !strings.Contains(buf.String(), "No logs found") {
		t.Errorf("empty table:\n%s", buf.String())
	}

	buf.Reset()
	WriteTaskLogs(&buf, []db.LogRow{{ID: "backup", EndTime: time.Now(), ElapsedMs: 1500}})
	out := buf.String()
	for _, want := range []string{"TASK ID", "COMPLETION TIME", "DURATION", "backup", "1.50s"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
}
