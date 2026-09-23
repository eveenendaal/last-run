package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eveenendaal/last-run/internal/apperr"
	"github.com/eveenendaal/last-run/internal/db"
	"github.com/eveenendaal/last-run/internal/model"
	"github.com/eveenendaal/last-run/internal/tui"
)

// runCLI executes the command tree against the database at dbPath, feeding
// stdin to the command, and returns everything written to stdout.
func runCLI(t *testing.T, dbPath, stdin string, args ...string) (string, error) {
	t.Helper()
	app := &appContext{}
	defer app.close()
	root := newRootCmd(app, "test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(append([]string{"--db-path", dbPath}, args...))
	err := root.Execute()
	return out.String(), err
}

// newDBPath returns a fresh database path and isolates the user config file.
func newDBPath(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	return filepath.Join(t.TempDir(), "data.db")
}

// withDB opens the database at path directly, to seed or inspect state.
func withDB(t *testing.T, path string, fn func(*sql.DB)) {
	t.Helper()
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := db.InitDB(conn); err != nil {
		t.Fatal(err)
	}
	fn(conn)
}

// seedRun records a completed run of id that finished `ago` in the past.
func seedRun(t *testing.T, conn *sql.DB, id string, ago time.Duration) {
	t.Helper()
	task, _, err := model.Ensure(conn, id)
	if err != nil {
		t.Fatal(err)
	}
	end := time.Now().UTC().Add(-ago)
	start := end.Add(-time.Second)
	task.StartTime, task.LastRun = &start, &end
	if err := task.Update(conn); err != nil {
		t.Fatal(err)
	}
}

func logCount(t *testing.T, path string) int {
	t.Helper()
	var n int
	withDB(t, path, func(conn *sql.DB) {
		logs, err := db.GetTaskLogs(conn, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		n = len(logs)
	})
	return n
}

func TestShouldRunTask(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		ago, threshold time.Duration
		wantDue        bool
		wantMsg        string
	}{
		{10 * time.Hour, 24 * time.Hour, false, "10h"},
		{30 * time.Hour, 24 * time.Hour, true, "1d6h"},
		{24 * time.Hour, 24 * time.Hour, true, "Task is due"},
		{6 * 24 * time.Hour, 7 * 24 * time.Hour, false, "6d"},
		{8 * 24 * time.Hour, 7 * 24 * time.Hour, true, "8d"},
	}
	for _, c := range cases {
		due, msg := ShouldRunTask(now.Add(-c.ago), c.threshold)
		if due != c.wantDue {
			t.Errorf("ShouldRunTask(%v ago, %v) due = %v, want %v", c.ago, c.threshold, due, c.wantDue)
		}
		wantPrefix := "Task is not due yet"
		if c.wantDue {
			wantPrefix = "Task is due"
		}
		if !strings.HasPrefix(msg, wantPrefix) || !strings.Contains(msg, c.wantMsg) {
			t.Errorf("ShouldRunTask(%v ago, %v) msg = %q, want prefix %q containing %q", c.ago, c.threshold, msg, wantPrefix, c.wantMsg)
		}
	}
}

func TestStartDoneWorkflow(t *testing.T) {
	path := newDBPath(t)

	out, err := runCLI(t, path, "", "start", "--id", "backup")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if !strings.Contains(out, "No record found for task ID: backup") || !strings.Contains(out, "started at") {
		t.Errorf("start output = %q", out)
	}

	for _, cmd := range []string{"done", "update"} {
		out, err = runCLI(t, path, "", cmd, "-i", "backup")
		if err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
		if !strings.Contains(out, "finished at") {
			t.Errorf("%s output = %q", cmd, out)
		}
	}
	// start_time is kept after done, so each done records a log entry.
	if n := logCount(t, path); n != 2 {
		t.Errorf("log entries = %d, want 2", n)
	}

	withDB(t, path, func(conn *sql.DB) {
		task, err := model.Select(conn, "backup")
		if err != nil || task == nil || task.LastRun == nil {
			t.Fatalf("task after done = %+v, %v", task, err)
		}
	})
}

func TestQuietSuppressesOutput(t *testing.T) {
	path := newDBPath(t)
	for _, args := range [][]string{
		{"--quiet", "start", "-i", "q"},
		{"-q", "done", "-i", "q"},
		{"-q", "logs"},
		{"-q", "status", "--json"},
	} {
		out, err := runCLI(t, path, "", args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if out != "" {
			t.Errorf("%v printed %q, want nothing", args, out)
		}
	}
}

func TestMissingTaskID(t *testing.T) {
	path := newDBPath(t)
	for _, cmd := range []string{"start", "done", "check", "clear", "delete"} {
		if _, err := runCLI(t, path, "", cmd); !errors.Is(err, apperr.ErrMissingTaskID) {
			t.Errorf("%s without --id: err = %v, want ErrMissingTaskID", cmd, err)
		}
	}
}

func TestCheck(t *testing.T) {
	path := newDBPath(t)

	out, err := runCLI(t, path, "", "check", "-i", "nightly")
	if !errors.Is(err, ErrTaskDue) || !strings.Contains(out, "does not exist yet") {
		t.Errorf("check missing task: (%q, %v), want ErrTaskDue", out, err)
	}

	if _, err := runCLI(t, path, "", "start", "-i", "nightly"); err != nil {
		t.Fatal(err)
	}
	out, err = runCLI(t, path, "", "check", "-i", "nightly")
	if !errors.Is(err, ErrTaskDue) || !strings.Contains(out, "no recorded last run") {
		t.Errorf("check never-finished task: (%q, %v), want ErrTaskDue", out, err)
	}

	withDB(t, path, func(conn *sql.DB) { seedRun(t, conn, "nightly", 2*time.Hour) })

	out, err = runCLI(t, path, "", "check", "-i", "nightly", "-d", "24h")
	if err != nil || !strings.Contains(out, "not due yet") {
		t.Errorf("check within threshold: (%q, %v), want not due", out, err)
	}
	out, err = runCLI(t, path, "", "check", "-i", "nightly", "--duration", "1h")
	if !errors.Is(err, ErrTaskDue) || !strings.Contains(out, "Task is due") {
		t.Errorf("check past threshold: (%q, %v), want ErrTaskDue", out, err)
	}

	// The last threshold is persisted for the status view.
	withDB(t, path, func(conn *sql.DB) {
		tasks, _ := db.GetAllTasks(conn, new("nightly"))
		if len(tasks) != 1 || tasks[0].Duration == nil || *tasks[0].Duration != 3600 {
			t.Errorf("stored duration = %+v, want 3600", tasks)
		}
	})

	var parseErr *apperr.DurationParseError
	if _, err := runCLI(t, path, "", "check", "-i", "nightly", "-d", "soon"); !errors.As(err, &parseErr) {
		t.Errorf("check bad duration: err = %v, want DurationParseError", err)
	}
}

func TestStatusJSON(t *testing.T) {
	path := newDBPath(t)
	withDB(t, path, func(conn *sql.DB) {
		seedRun(t, conn, "alpha", time.Hour)
		seedRun(t, conn, "beta", time.Hour)
	})

	out, err := runCLI(t, path, "", "status", "--json", "--id", "alpha")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Tasks []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if len(got.Tasks) != 1 || got.Tasks[0].ID != "alpha" || got.Tasks[0].Status != "ok" {
		t.Errorf("status --json --id alpha = %+v", got.Tasks)
	}
}

func TestStatusRejectsBadSort(t *testing.T) {
	path := newDBPath(t)
	if _, err := runCLI(t, path, "", "status", "--sort", "bogus"); err == nil || !strings.Contains(err.Error(), "invalid sort column") {
		t.Errorf("err = %v, want invalid sort column", err)
	}
}

func TestParseSortColumn(t *testing.T) {
	for name, want := range map[string]tui.SortCol{
		"task": tui.SortTask, "status": tui.SortStatus, "duration": tui.SortDuration,
		"elapsed": tui.SortElapsed, "last-run": tui.SortLastRun,
	} {
		if got, err := parseSortColumn(name); err != nil || got != want {
			t.Errorf("parseSortColumn(%q) = (%v, %v), want %v", name, got, err, want)
		}
	}
}

func TestLogs(t *testing.T) {
	path := newDBPath(t)

	out, err := runCLI(t, path, "", "logs")
	if err != nil || !strings.Contains(out, "No logs found") {
		t.Errorf("empty logs: (%q, %v)", out, err)
	}

	withDB(t, path, func(conn *sql.DB) {
		seedRun(t, conn, "alpha", time.Hour)
		seedRun(t, conn, "beta", time.Hour)
	})
	out, err = runCLI(t, path, "", "logs", "--id", "beta")
	if err != nil || !strings.Contains(out, "beta") || strings.Contains(out, "alpha") {
		t.Errorf("logs --id beta: (%q, %v)", out, err)
	}
}

func TestArchive(t *testing.T) {
	path := newDBPath(t)
	withDB(t, path, func(conn *sql.DB) {
		seedRun(t, conn, "old", 10*24*time.Hour)
		seedRun(t, conn, "new", time.Hour)
	})

	out, err := runCLI(t, path, "n\n", "archive", "--older-than", "7d")
	if err != nil || !strings.Contains(out, "Cancelled") {
		t.Errorf("declined archive: (%q, %v)", out, err)
	}
	if n := logCount(t, path); n != 2 {
		t.Fatalf("after cancel: %d logs, want 2", n)
	}

	out, err = runCLI(t, path, "y\n", "archive", "-o", "7d")
	if err != nil || !strings.Contains(out, "Deleted 1 log entry") {
		t.Errorf("confirmed archive: (%q, %v)", out, err)
	}
	if n := logCount(t, path); n != 1 {
		t.Fatalf("after archive: %d logs, want 1", n)
	}

	out, err = runCLI(t, path, "", "archive", "-o", "7d", "--yes")
	if err != nil || !strings.Contains(out, "No log entries found older than 7d") {
		t.Errorf("nothing to archive: (%q, %v)", out, err)
	}

	var parseErr *apperr.DurationParseError
	if _, err := runCLI(t, path, "", "archive", "-o", "later"); !errors.As(err, &parseErr) {
		t.Errorf("bad --older-than: err = %v, want DurationParseError", err)
	}
}

func TestArchiveDefaultsWhenRetentionOff(t *testing.T) {
	path := newDBPath(t)
	withDB(t, path, func(conn *sql.DB) {
		if err := db.SetLogRetention(conn, "off"); err != nil {
			t.Fatal(err)
		}
		seedRun(t, conn, "task", 40*24*time.Hour)
		seedRun(t, conn, "task", 24*time.Hour)
	})

	out, err := runCLI(t, path, "", "archive", "--yes", "--id", "task")
	if err != nil || !strings.Contains(out, "for task") || !strings.Contains(out, "Deleted 1 log entry") {
		t.Errorf("archive with retention off: (%q, %v)", out, err)
	}
	if n := logCount(t, path); n != 1 {
		t.Errorf("remaining logs = %d, want 1", n)
	}
}

func TestAutoArchiveOnDone(t *testing.T) {
	cases := []struct {
		retention string // "" leaves the setting unset (30d default)
		wantLogs  int    // after `done`, which adds one entry
	}{
		{"", 2},    // 40d-old entry removed, 5d-old kept
		{"1d", 1},  // both seeded entries removed
		{"off", 3}, // nothing removed
	}
	for _, c := range cases {
		path := newDBPath(t)
		withDB(t, path, func(conn *sql.DB) {
			if c.retention != "" {
				if err := db.SetLogRetention(conn, c.retention); err != nil {
					t.Fatal(err)
				}
			}
			seedRun(t, conn, "job", 40*24*time.Hour)
			seedRun(t, conn, "job", 5*24*time.Hour)
		})
		if _, err := runCLI(t, path, "", "done", "-i", "job"); err != nil {
			t.Fatal(err)
		}
		if n := logCount(t, path); n != c.wantLogs {
			t.Errorf("retention %q: %d logs after done, want %d", c.retention, n, c.wantLogs)
		}
	}
}

func TestSetRetention(t *testing.T) {
	path := newDBPath(t)

	out, err := runCLI(t, path, "", "set-retention", "60d")
	if err != nil || !strings.Contains(out, "Log retention set to") {
		t.Errorf("set-retention 60d: (%q, %v)", out, err)
	}
	out, err = runCLI(t, path, "", "set-retention", "off")
	if err != nil || !strings.Contains(out, "disabled") {
		t.Errorf("set-retention off: (%q, %v)", out, err)
	}
	if _, err := runCLI(t, path, "", "set-retention", "forever"); err == nil {
		t.Error("set-retention forever: want error")
	}
	if _, err := runCLI(t, path, "", "set-retention"); err == nil {
		t.Error("set-retention without argument: want error")
	}
	withDB(t, path, func(conn *sql.DB) {
		if d, _ := db.LogRetention(conn); d != 0 {
			t.Errorf("retention = %v, want off", d)
		}
	})
}

func TestClearDeleteReset(t *testing.T) {
	path := newDBPath(t)
	withDB(t, path, func(conn *sql.DB) {
		seedRun(t, conn, "a", time.Hour)
		seedRun(t, conn, "b", time.Hour)
	})

	if out, _ := runCLI(t, path, "", "clear", "-i", "missing"); !strings.Contains(out, "does not exist") {
		t.Errorf("clear missing: %q", out)
	}
	if out, err := runCLI(t, path, "", "clear", "-i", "a"); err != nil || !strings.Contains(out, "cleared") {
		t.Errorf("clear a: (%q, %v)", out, err)
	}
	withDB(t, path, func(conn *sql.DB) {
		task, _ := model.Select(conn, "a")
		if task == nil || task.LastRun != nil || task.StartTime != nil {
			t.Errorf("task a after clear = %+v", task)
		}
	})

	if out, err := runCLI(t, path, "", "delete", "-i", "b"); err != nil || !strings.Contains(out, "deleted. 1 log entries removed") {
		t.Errorf("delete b: (%q, %v)", out, err)
	}
	if out, _ := runCLI(t, path, "", "delete", "-i", "b"); !strings.Contains(out, "No task found") {
		t.Errorf("delete b again: %q", out)
	}

	if out, err := runCLI(t, path, "", "reset"); err != nil || !strings.Contains(out, "rebuilt") {
		t.Errorf("reset: (%q, %v)", out, err)
	}
	withDB(t, path, func(conn *sql.DB) {
		if tasks, _ := db.GetAllTasks(conn, nil); len(tasks) != 0 {
			t.Errorf("tasks after reset = %+v, want none", tasks)
		}
	})
	if n := logCount(t, path); n != 1 {
		t.Errorf("logs after reset = %d, want 1 (history is kept)", n)
	}
}
