package db_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eveenendaal/last-run/internal/apperr"
	"github.com/eveenendaal/last-run/internal/db"
	"github.com/eveenendaal/last-run/internal/model"
)

func TestDatabaseInitialization(t *testing.T) {
	database := newTestDB(t)

	rows, err := database.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	found := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		found[name] = true
	}

	for _, table := range []string{"tasks", "task_log", "settings"} {
		if !found[table] {
			t.Errorf("expected table %q to exist", table)
		}
	}
}

func TestTaskCRUDOperations(t *testing.T) {
	database := newTestDB(t)

	task := makeTask("test_task")
	if err := task.Insert(database); err != nil {
		t.Fatal(err)
	}

	fetched, err := model.Select(database, "test_task")
	if err != nil || fetched == nil {
		t.Fatalf("select: %v (task=%v)", err, fetched)
	}
	if fetched.ID != "test_task" {
		t.Errorf("ID = %q, want test_task", fetched.ID)
	}

	now := time.Now().UTC()
	task.LastRun = &now
	if err := task.Update(database); err != nil {
		t.Fatal(err)
	}

	updated, err := model.Select(database, "test_task")
	if err != nil {
		t.Fatal(err)
	}
	if updated.LastRun == nil {
		t.Error("expected LastRun to be set after update")
	}
}

func TestGetAllTasks(t *testing.T) {
	database := newTestDB(t)

	for _, id := range []string{"task1", "task2"} {
		if err := makeTask(id).Insert(database); err != nil {
			t.Fatal(err)
		}
	}

	tasks, err := db.GetAllTasks(database, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Errorf("len(tasks) = %d, want 2", len(tasks))
	}
}

func TestGetTaskLogs(t *testing.T) {
	database := newTestDB(t)

	task := makeTask("task1")
	if err := task.Insert(database); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	task.StartTime = &now
	task.LastRun = &now
	if err := task.Update(database); err != nil {
		t.Fatal(err)
	}

	logs, err := db.GetTaskLogs(database, new("task1"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Errorf("len(logs) = %d, want 1", len(logs))
	}
}

func TestResetCommand(t *testing.T) {
	database := newTestDB(t)

	for _, id := range []string{"reset_test_1", "reset_test_2"} {
		if err := makeTask(id).Insert(database); err != nil {
			t.Fatal(err)
		}
	}

	before, _ := db.GetAllTasks(database, nil)
	if len(before) != 2 {
		t.Fatalf("len(before) = %d, want 2", len(before))
	}

	if err := db.CleanDB(database); err != nil {
		t.Fatal(err)
	}

	after, _ := db.GetAllTasks(database, nil)
	if len(after) != 0 {
		t.Errorf("len(after) = %d, want 0", len(after))
	}

	if err := makeTask("post_reset_task").Insert(database); err != nil {
		t.Fatal(err)
	}
	final, _ := db.GetAllTasks(database, nil)
	if len(final) != 1 || final[0].ID != "post_reset_task" {
		t.Errorf("final tasks = %+v, want one post_reset_task", final)
	}
}

func TestDeleteCommand(t *testing.T) {
	database := newTestDB(t)

	task := makeTask("delete_test")
	if err := task.Insert(database); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC()
	task.StartTime = &start
	if err := task.Update(database); err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)
	last := time.Now().UTC()
	task.LastRun = &last
	if err := task.Update(database); err != nil {
		t.Fatal(err)
	}

	tasksBefore, _ := db.GetAllTasks(database, new("delete_test"))
	logsBefore, _ := db.GetTaskLogs(database, new("delete_test"), 10)
	if len(tasksBefore) != 1 || len(logsBefore) != 1 {
		t.Fatalf("before: tasks=%d logs=%d, want 1/1", len(tasksBefore), len(logsBefore))
	}

	logsDeleted, err := db.DeleteTaskLogs(database, "delete_test")
	if err != nil {
		t.Fatal(err)
	}
	if logsDeleted != 1 {
		t.Errorf("logsDeleted = %d, want 1", logsDeleted)
	}

	logsAfter, _ := db.GetTaskLogs(database, new("delete_test"), 10)
	tasksAfterLogDelete, _ := db.GetAllTasks(database, new("delete_test"))
	if len(logsAfter) != 0 || len(tasksAfterLogDelete) != 1 {
		t.Errorf("after log delete: logs=%d tasks=%d, want 0/1", len(logsAfter), len(tasksAfterLogDelete))
	}

	taskDeleted, err := db.DeleteTask(database, "delete_test")
	if err != nil {
		t.Fatal(err)
	}
	if taskDeleted != 1 {
		t.Errorf("taskDeleted = %d, want 1", taskDeleted)
	}

	tasksAfter, _ := db.GetAllTasks(database, new("delete_test"))
	if len(tasksAfter) != 0 {
		t.Errorf("tasksAfter = %d, want 0", len(tasksAfter))
	}

	nonExistent, err := db.DeleteTask(database, "non_existent_task")
	if err != nil {
		t.Fatal(err)
	}
	if nonExistent != 0 {
		t.Errorf("deleting non-existent task = %d, want 0", nonExistent)
	}
}

func TestSettingsCRUD(t *testing.T) {
	database := newTestDB(t)

	if _, ok, _ := db.GetSetting(database, "log_retention"); ok {
		t.Error("expected log_retention to be unset initially")
	}

	if err := db.SetSetting(database, "log_retention", "30d"); err != nil {
		t.Fatal(err)
	}
	if val, ok, _ := db.GetSetting(database, "log_retention"); !ok || val != "30d" {
		t.Errorf("got (%q, %v), want (30d, true)", val, ok)
	}

	if err := db.SetSetting(database, "log_retention", "60d"); err != nil {
		t.Fatal(err)
	}
	if val, ok, _ := db.GetSetting(database, "log_retention"); !ok || val != "60d" {
		t.Errorf("got (%q, %v), want (60d, true)", val, ok)
	}

	if err := db.SetSetting(database, "other_key", "some_value"); err != nil {
		t.Fatal(err)
	}
	all, _ := db.GetAllSettings(database)
	if len(all) != 2 {
		t.Fatalf("len(all) = %d, want 2", len(all))
	}
	wantPairs := map[string]string{"log_retention": "60d", "other_key": "some_value"}
	for _, s := range all {
		if wantPairs[s.Key] != s.Value {
			t.Errorf("setting %q = %q, want %q", s.Key, s.Value, wantPairs[s.Key])
		}
	}
}

func TestLogRetention(t *testing.T) {
	database := newTestDB(t)

	cases := []struct {
		stored string // "" leaves the setting unset
		want   time.Duration
	}{
		{"", db.DefaultLogRetention},
		{"30d", 30 * 24 * time.Hour},
		{"2w", 14 * 24 * time.Hour},
		{"12h", 12 * time.Hour},
		{"off", 0},
		{"OFF", 0},
		{"0", 0},
		{"not_a_duration", db.DefaultLogRetention},
	}
	for _, c := range cases {
		if c.stored != "" {
			if err := db.SetSetting(database, "log_retention", c.stored); err != nil {
				t.Fatal(err)
			}
		}
		got, err := db.LogRetention(database)
		if err != nil || got != c.want {
			t.Errorf("LogRetention with %q = (%v, %v), want (%v, nil)", c.stored, got, err, c.want)
		}
	}
}

func TestSetLogRetention(t *testing.T) {
	database := newTestDB(t)

	for in, want := range map[string]string{"60d": "60d", "off": "off", "Off": "off", "0": "off"} {
		if err := db.SetLogRetention(database, in); err != nil {
			t.Fatalf("SetLogRetention(%q): %v", in, err)
		}
		if got, _, _ := db.GetSetting(database, "log_retention"); got != want {
			t.Errorf("SetLogRetention(%q) stored %q, want %q", in, got, want)
		}
	}

	var parseErr *apperr.DurationParseError
	if err := db.SetLogRetention(database, "10x"); !errors.As(err, &parseErr) {
		t.Errorf("SetLogRetention(10x) err = %v, want DurationParseError", err)
	}
}

func TestTaskStatus(t *testing.T) {
	now := time.Now().UTC()
	at := func(d time.Duration) *time.Time { return new(now.Add(-d)) }
	hour := int64(3600)

	cases := []struct {
		name        string
		task        db.TaskStatus
		wantStatus  string
		wantElapsed time.Duration
		wantOK      bool
	}{
		{"never run", db.TaskStatus{}, db.StatusUnknown, 0, false},
		{"running", db.TaskStatus{StartTime: at(5 * time.Minute)}, db.StatusRunning, 5 * time.Minute, true},
		{"done without threshold", db.TaskStatus{StartTime: at(3 * time.Hour), LastRun: at(2 * time.Hour)}, db.StatusOK, time.Hour, true},
		{"done within threshold", db.TaskStatus{LastRun: at(30 * time.Minute), Duration: &hour}, db.StatusOK, 0, false},
		{"overdue", db.TaskStatus{LastRun: at(2 * time.Hour), Duration: &hour}, db.StatusDue, 0, false},
		{"start after last run", db.TaskStatus{StartTime: at(time.Minute), LastRun: at(time.Hour)}, db.StatusOK, 0, false},
	}
	for _, c := range cases {
		if got := c.task.Status(now); got != c.wantStatus {
			t.Errorf("%s: Status = %q, want %q", c.name, got, c.wantStatus)
		}
		if got, ok := c.task.Elapsed(now); got != c.wantElapsed || ok != c.wantOK {
			t.Errorf("%s: Elapsed = (%v, %v), want (%v, %v)", c.name, got, ok, c.wantElapsed, c.wantOK)
		}
	}
}

func TestOldLogsFilteredByTask(t *testing.T) {
	database := newTestDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour)
	for _, id := range []string{"a", "b"} {
		task := makeTask(id)
		if err := task.Insert(database); err != nil {
			t.Fatal(err)
		}
		task.StartTime, task.LastRun = &old, &old
		if err := task.Update(database); err != nil {
			t.Fatal(err)
		}
	}

	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	if n, err := db.CountOldLogs(database, cutoff, new("a")); err != nil || n != 1 {
		t.Errorf("CountOldLogs(a) = (%d, %v), want (1, nil)", n, err)
	}
	if n, err := db.CountOldLogs(database, cutoff, nil); err != nil || n != 2 {
		t.Errorf("CountOldLogs(all) = (%d, %v), want (2, nil)", n, err)
	}
	if n, err := db.DeleteOldLogs(database, cutoff, new("a")); err != nil || n != 1 {
		t.Errorf("DeleteOldLogs(a) = (%d, %v), want (1, nil)", n, err)
	}
	logs, _ := db.GetTaskLogs(database, nil, 0)
	if len(logs) != 1 || logs[0].ID != "b" {
		t.Errorf("remaining logs = %+v, want only b", logs)
	}
}

func TestGetTaskLogsLimitAndOrder(t *testing.T) {
	database := newTestDB(t)
	task := makeTask("limited")
	if err := task.Insert(database); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Add(-time.Hour)
	for i := range 3 {
		start, end := base.Add(time.Duration(i)*time.Minute), base.Add(time.Duration(i)*time.Minute+time.Second)
		task.StartTime, task.LastRun = &start, &end
		if err := task.Update(database); err != nil {
			t.Fatal(err)
		}
	}

	logs, err := db.GetTaskLogs(database, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || !logs[0].EndTime.After(logs[1].EndTime) {
		t.Errorf("GetTaskLogs limit 2 = %+v, want 2 newest-first entries", logs)
	}
	if all, _ := db.GetTaskLogs(database, nil, 0); len(all) != 3 {
		t.Errorf("GetTaskLogs limit 0 = %d entries, want 3", len(all))
	}

	entries, err := db.GetTaskLogEntries(database, "limited")
	if err != nil || len(entries) != 3 {
		t.Fatalf("GetTaskLogEntries = (%d, %v), want 3", len(entries), err)
	}
	if n, err := db.DeleteTaskLogEntry(database, "limited", entries[0].Raw); err != nil || n != 1 {
		t.Errorf("DeleteTaskLogEntry = (%d, %v), want (1, nil)", n, err)
	}
	if entries[0].ElapsedMs != 1000 {
		t.Errorf("ElapsedMs = %d, want 1000", entries[0].ElapsedMs)
	}
}

func TestOpenCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "data.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()
	if err := db.InitDB(database); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("database file not created: %v", err)
	}
}

func TestCopyDatabase(t *testing.T) {
	src := newTestDB(t)
	if err := makeTask("original_task").Insert(src); err != nil {
		t.Fatal(err)
	}

	dstPath := filepath.Join(t.TempDir(), "copy.db")
	if err := db.CopyDatabase(src, dstPath); err != nil {
		t.Fatalf("CopyDatabase: %v", err)
	}

	dst, err := db.Open(dstPath)
	if err != nil {
		t.Fatalf("open copy: %v", err)
	}
	defer dst.Close()
	if err := db.InitDB(dst); err != nil {
		t.Fatal(err)
	}

	tasks, err := db.GetAllTasks(dst, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "original_task" {
		t.Errorf("copy tasks = %+v, want [original_task]", tasks)
	}
}

func TestCopyDatabaseFailsIfExists(t *testing.T) {
	src := newTestDB(t)
	dstPath := filepath.Join(t.TempDir(), "existing.db")
	// Pre-create destination file.
	if err := os.WriteFile(dstPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := db.CopyDatabase(src, dstPath); err == nil {
		t.Error("expected error when destination already exists")
	}
}

func TestSetGetCustomDBPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if path, err := db.GetCustomDBPath(); err != nil || path != "" {
		t.Errorf("initial GetCustomDBPath = (%q, %v), want (\"\", nil)", path, err)
	}

	want := "/custom/data.db"
	if err := db.SetCustomDBPath(want); err != nil {
		t.Fatalf("SetCustomDBPath: %v", err)
	}
	if got, _ := db.GetCustomDBPath(); got != want {
		t.Errorf("GetCustomDBPath = %q, want %q", got, want)
	}

	// Clear to revert to default.
	if err := db.SetCustomDBPath(""); err != nil {
		t.Fatalf("SetCustomDBPath clear: %v", err)
	}
	if got, _ := db.GetCustomDBPath(); got != "" {
		t.Errorf("after clear GetCustomDBPath = %q, want \"\"", got)
	}
}

func TestResolveDBPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir()) // ensures XDG_DATA_HOME resolves cleanly

	// Override takes priority.
	got, err := db.ResolveDBPath("/explicit/path.db")
	if err != nil || got != "/explicit/path.db" {
		t.Errorf("override: got (%q, %v), want (/explicit/path.db, nil)", got, err)
	}

	// Config file takes priority over XDG default when set.
	if err := db.SetCustomDBPath("/from/config.db"); err != nil {
		t.Fatal(err)
	}
	got, err = db.ResolveDBPath("")
	if err != nil || got != "/from/config.db" {
		t.Errorf("config: got (%q, %v), want (/from/config.db, nil)", got, err)
	}

	// Clear config → falls back to XDG default.
	if err := db.SetCustomDBPath(""); err != nil {
		t.Fatal(err)
	}
	got, err = db.ResolveDBPath("")
	if err != nil {
		t.Fatalf("xdg default: %v", err)
	}
	if got == "" || got == "/from/config.db" {
		t.Errorf("xdg default: unexpected path %q", got)
	}
}
