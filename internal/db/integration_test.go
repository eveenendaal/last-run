package db_test

import (
	"testing"
	"time"

	"github.com/eveenendaal/last-run/internal/cli"
	"github.com/eveenendaal/last-run/internal/db"
)

func TestCompleteTaskWorkflow(t *testing.T) {
	database := newTestDB(t)

	taskID := "workflow_test"
	task := makeTask(taskID)
	if err := task.Insert(database); err != nil {
		t.Fatal(err)
	}

	tasks, _ := db.GetAllTasks(database, ptr(taskID))
	if len(tasks) != 1 || tasks[0].ID != taskID || tasks[0].LastRun != nil {
		t.Fatalf("after insert: %+v", tasks)
	}

	start := time.Now().UTC()
	task.StartTime = &start
	if err := task.Update(database); err != nil {
		t.Fatal(err)
	}

	afterStart, _ := db.GetAllTasks(database, ptr(taskID))
	if afterStart[0].LastRun != nil {
		t.Error("expected LastRun cleared after start")
	}

	time.Sleep(100 * time.Millisecond)
	last := time.Now().UTC()
	task.LastRun = &last
	if err := task.Update(database); err != nil {
		t.Fatal(err)
	}

	updated, _ := db.GetAllTasks(database, ptr(taskID))
	if updated[0].LastRun == nil {
		t.Fatal("expected LastRun set after done")
	}
	lastRun := *updated[0].LastRun

	logs, _ := db.GetTaskLogs(database, ptr(taskID), 10)
	if len(logs) != 1 || logs[0].ID != taskID || logs[0].ElapsedMs <= 0 {
		t.Fatalf("logs = %+v, want one entry with elapsed > 0", logs)
	}

	if shouldRun, _ := cli.ShouldRunTask(lastRun, 24*time.Hour); shouldRun {
		t.Error("task should not be due immediately after completion")
	}

	logsDeleted, _ := db.DeleteTaskLogs(database, taskID)
	if logsDeleted != 1 {
		t.Errorf("logsDeleted = %d, want 1", logsDeleted)
	}
	logsAfter, _ := db.GetTaskLogs(database, ptr(taskID), 10)
	if len(logsAfter) != 0 {
		t.Errorf("logsAfter = %d, want 0", len(logsAfter))
	}

	taskDeleted, _ := db.DeleteTask(database, taskID)
	if taskDeleted != 1 {
		t.Errorf("taskDeleted = %d, want 1", taskDeleted)
	}
	final, _ := db.GetAllTasks(database, ptr(taskID))
	if len(final) != 0 {
		t.Errorf("final = %d, want 0", len(final))
	}
}

func TestMultipleTaskManagement(t *testing.T) {
	database := newTestDB(t)

	taskIDs := []string{"daily_task", "weekly_task", "monthly_task"}
	for _, id := range taskIDs {
		if err := makeTask(id).Insert(database); err != nil {
			t.Fatal(err)
		}
	}

	all, _ := db.GetAllTasks(database, nil)
	if len(all) != 3 {
		t.Fatalf("len(all) = %d, want 3", len(all))
	}

	for _, id := range taskIDs {
		task := makeTask(id)
		start := time.Now().UTC()
		task.StartTime = &start
		if err := task.Update(database); err != nil {
			t.Fatal(err)
		}
	}

	// last_run is cleared for all tasks after a start.
	for _, id := range taskIDs {
		status, _ := db.GetAllTasks(database, ptr(id))
		if status[0].LastRun != nil {
			t.Errorf("task %q: expected LastRun nil", id)
		}
	}

	for _, id := range taskIDs {
		if _, err := db.DeleteTaskLogs(database, id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.DeleteTask(database, id); err != nil {
			t.Fatal(err)
		}
	}

	final, _ := db.GetAllTasks(database, nil)
	if len(final) != 0 {
		t.Errorf("final = %d, want 0", len(final))
	}
}

func TestAutoArchiveOnDone(t *testing.T) {
	database := newTestDB(t)

	if err := db.SetLogRetention(database, "1h"); err != nil {
		t.Fatal(err)
	}

	task := makeTask("auto_archive_test")
	if err := task.Insert(database); err != nil {
		t.Fatal(err)
	}
	twoHoursAgo := time.Now().UTC().Add(-2 * time.Hour)
	task.StartTime = &twoHoursAgo
	task.LastRun = &twoHoursAgo
	if err := task.Update(database); err != nil {
		t.Fatal(err)
	}

	logs, _ := db.GetTaskLogs(database, ptr("auto_archive_test"), 10)
	if len(logs) != 1 {
		t.Fatalf("len(logs) = %d, want 1", len(logs))
	}

	cutoff := time.Now().UTC().Add(-1 * time.Hour)
	deleted, _ := db.DeleteOldLogs(database, cutoff, nil)
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	logs, _ = db.GetTaskLogs(database, ptr("auto_archive_test"), 10)
	if len(logs) != 0 {
		t.Errorf("len(logs) = %d, want 0", len(logs))
	}
}

func TestArchiveDefaultFallsBackWhenRetentionOff(t *testing.T) {
	database := newTestDB(t)

	if err := db.SetLogRetention(database, "off"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.GetLogRetentionSeconds(database); ok {
		t.Fatal("expected retention off to return ok=false")
	}

	task := makeTask("archive_off_test")
	if err := task.Insert(database); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-40 * 24 * time.Hour)
	task.StartTime = &old
	task.LastRun = &old
	if err := task.Update(database); err != nil {
		t.Fatal(err)
	}
	recent := time.Now().UTC().Add(-24 * time.Hour)
	task.StartTime = &recent
	task.LastRun = &recent
	if err := task.Update(database); err != nil {
		t.Fatal(err)
	}

	seconds, ok, _ := db.GetLogRetentionSeconds(database)
	if !ok {
		seconds = 30 * 24 * 3600
	}
	cutoff := time.Now().UTC().Add(-time.Duration(seconds) * time.Second)

	deleted, _ := db.DeleteOldLogs(database, cutoff, nil)
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	logs, _ := db.GetTaskLogs(database, ptr("archive_off_test"), 10)
	if len(logs) != 1 {
		t.Errorf("len(logs) = %d, want 1", len(logs))
	}
}

func TestArchivePreservesRecentLogs(t *testing.T) {
	database := newTestDB(t)

	task := makeTask("preserve_test")
	if err := task.Insert(database); err != nil {
		t.Fatal(err)
	}

	old := time.Now().UTC().Add(-48 * time.Hour)
	task.StartTime = &old
	task.LastRun = &old
	if err := task.Update(database); err != nil {
		t.Fatal(err)
	}

	recent := time.Now().UTC().Add(-5 * time.Minute)
	task.StartTime = &recent
	task.LastRun = &recent
	if err := task.Update(database); err != nil {
		t.Fatal(err)
	}

	logs, _ := db.GetTaskLogs(database, ptr("preserve_test"), 10)
	if len(logs) != 2 {
		t.Fatalf("len(logs) = %d, want 2", len(logs))
	}

	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	deleted, _ := db.DeleteOldLogs(database, cutoff, nil)
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	logs, _ = db.GetTaskLogs(database, ptr("preserve_test"), 10)
	if len(logs) != 1 {
		t.Errorf("len(logs) = %d, want 1", len(logs))
	}
}
