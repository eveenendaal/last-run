package db_test

import (
	"testing"
	"time"

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

	logs, err := db.GetTaskLogs(database, ptr("task1"), 10)
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

	tasksBefore, _ := db.GetAllTasks(database, ptr("delete_test"))
	logsBefore, _ := db.GetTaskLogs(database, ptr("delete_test"), 10)
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

	logsAfter, _ := db.GetTaskLogs(database, ptr("delete_test"), 10)
	tasksAfterLogDelete, _ := db.GetAllTasks(database, ptr("delete_test"))
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

	tasksAfter, _ := db.GetAllTasks(database, ptr("delete_test"))
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

func TestLogRetentionSeconds(t *testing.T) {
	database := newTestDB(t)

	if _, ok, _ := db.GetLogRetentionSeconds(database); ok {
		t.Error("expected unset retention to return ok=false")
	}

	if err := db.SetLogRetention(database, "30d"); err != nil {
		t.Fatal(err)
	}
	if secs, ok, _ := db.GetLogRetentionSeconds(database); !ok || secs != 30*24*3600 {
		t.Errorf("got (%d, %v), want (%d, true)", secs, ok, 30*24*3600)
	}

	if err := db.SetLogRetention(database, "off"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.GetLogRetentionSeconds(database); ok {
		t.Error("expected 'off' retention to return ok=false")
	}

	if err := db.SetLogRetention(database, "0"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.GetLogRetentionSeconds(database); ok {
		t.Error("expected '0' retention to return ok=false")
	}

	if err := db.SetSetting(database, "log_retention", "not_a_duration"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.GetLogRetentionSeconds(database); ok {
		t.Error("expected invalid retention to return ok=false")
	}
}
