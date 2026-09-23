// Package model holds the in-memory Task representation and its persistence
// helpers.
package model

import (
	"database/sql"
	"errors"
	"time"

	"github.com/eveenendaal/last-run/internal/format"
)

// Task is the in-memory representation of a row from the `tasks` table.
type Task struct {
	ID        string
	LastRun   *time.Time
	StartTime *time.Time
}

func rfc3339OrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return format.FormatRFC3339(*t)
}

// Update writes the task's timing fields back to `tasks`, and—when both
// start_time and last_run are set—appends an entry to `task_log` with the
// computed elapsed time in milliseconds.
func (t *Task) Update(db *sql.DB) error {
	if _, err := db.Exec(
		"UPDATE tasks SET last_run = ?, start_time = ? WHERE id = ?",
		rfc3339OrNil(t.LastRun), rfc3339OrNil(t.StartTime), t.ID,
	); err != nil {
		return err
	}

	if t.StartTime != nil && t.LastRun != nil {
		elapsedMs := t.LastRun.Sub(*t.StartTime).Milliseconds()
		if _, err := db.Exec(
			"INSERT INTO task_log (id, end_time, elapsed_time) VALUES (?, ?, ?)",
			t.ID, format.FormatRFC3339(*t.LastRun), elapsedMs,
		); err != nil {
			return err
		}
	}

	return nil
}

// Insert creates a new row in `tasks`.
func (t *Task) Insert(db *sql.DB) error {
	_, err := db.Exec(
		"INSERT INTO tasks (id, last_run, start_time) VALUES (?, ?, ?)",
		t.ID, rfc3339OrNil(t.LastRun), rfc3339OrNil(t.StartTime),
	)
	return err
}

// Select fetches a single task by ID, returning nil if it does not exist.
func Select(db *sql.DB, id string) (*Task, error) {
	row := db.QueryRow("SELECT id, last_run, start_time FROM tasks WHERE id = ?", id)

	var taskID string
	var lastRun, startTime sql.NullString
	if err := row.Scan(&taskID, &lastRun, &startTime); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &Task{
		ID:        taskID,
		LastRun:   format.ParseRFC3339Opt(lastRun.String),
		StartTime: format.ParseRFC3339Opt(startTime.String),
	}, nil
}

// Ensure returns the existing task, or creates and inserts a new empty one if
// it does not exist. created reports whether a new task was inserted.
func Ensure(db *sql.DB, id string) (task *Task, created bool, err error) {
	task, err = Select(db, id)
	if err != nil || task != nil {
		return task, false, err
	}
	task = &Task{ID: id}
	if err := task.Insert(db); err != nil {
		return nil, false, err
	}
	return task, true, nil
}
