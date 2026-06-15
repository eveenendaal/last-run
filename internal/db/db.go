// Package db owns the SQLite connection, schema, and all typed CRUD helpers,
// mirroring the original Rust db.rs. It uses the pure-Go modernc.org/sqlite
// driver so binaries are statically linked and cross-compile without cgo.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/eveenendaal/last-run/internal/apperr"
	"github.com/eveenendaal/last-run/internal/format"

	_ "modernc.org/sqlite"
)

// TaskStatus is a task row with parsed timestamps and optional duration.
type TaskStatus struct {
	ID        string
	LastRun   *time.Time
	StartTime *time.Time
	Duration  *int64
}

// LogRow is a single task_log entry with a parsed end time.
type LogRow struct {
	ID        string
	EndTime   time.Time
	ElapsedMs int64
}

// LogEntry is a task_log entry that also retains the raw end_time string, used
// as the primary key when deleting individual entries from the history view.
type LogEntry struct {
	Raw       string
	EndTime   time.Time
	ElapsedMs int64
}

// Setting is a single key/value pair from the settings table.
type Setting struct {
	Key   string
	Value string
}

// InitDB creates the schema idempotently and ensures the `duration` column
// exists on older databases.
func InitDB(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			last_run TEXT,
			start_time TEXT,
			duration INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS task_log (
			id TEXT,
			end_time TEXT,
			elapsed_time INTEGER,
			PRIMARY KEY (id, end_time)
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}

	// Best-effort: add the `duration` column if it predates that change.
	// Ignore the error when the column already exists.
	_, _ = db.Exec("ALTER TABLE tasks ADD COLUMN duration INTEGER")

	return nil
}

// Open opens (or creates) a SQLite database at the given path. The connection
// pool is capped at one to avoid "database is locked" contention from within
// the same process. A 5-second busy timeout and WAL journal mode handle
// cross-process contention gracefully.
func Open(path string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	if _, err := database.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		return nil, err
	}
	if _, err := database.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return nil, err
	}
	return database, nil
}

// GetFileBasedConnection resolves the database path (honoring an override),
// migrates an old database if applicable, ensures the parent directory exists,
// and opens the connection.
func GetFileBasedConnection(dbPathOverride string) (*sql.DB, error) {
	dbPath, err := resolveDBPath(dbPathOverride)
	if err != nil {
		return nil, err
	}

	if parent := filepath.Dir(dbPath); parent != "" {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return nil, err
		}
	}

	return Open(dbPath)
}

func resolveDBPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return defaultDBPath()
}

func defaultDBPath() (string, error) {
	dataHome := xdg.DataHome
	if dataHome == "" {
		return "", apperr.ErrDataDirectoryNotFound
	}
	return filepath.Join(dataHome, "lastrun", "data.db"), nil
}

// GetTaskLogs returns recent log entries, optionally filtered by task ID. A
// limit of 0 means no limit. Entries are ordered newest first.
func GetTaskLogs(db *sql.DB, taskID *string, limit int) ([]LogRow, error) {
	query := "SELECT id, end_time, elapsed_time FROM task_log"
	if taskID != nil {
		query += " WHERE id = ?"
	}
	query += " ORDER BY end_time DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	var rows *sql.Rows
	var err error
	if taskID != nil {
		rows, err = db.Query(query, *taskID)
	} else {
		rows, err = db.Query(query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []LogRow
	for rows.Next() {
		var id, endTimeStr string
		var elapsed int64
		if err := rows.Scan(&id, &endTimeStr, &elapsed); err != nil {
			return nil, err
		}
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			return nil, fmt.Errorf("Date parse error: %w", err)
		}
		logs = append(logs, LogRow{ID: id, EndTime: endTime.UTC(), ElapsedMs: elapsed})
	}
	return logs, rows.Err()
}

// GetAllTasks returns all tasks (optionally filtered by ID), ordered by ID.
func GetAllTasks(db *sql.DB, taskID *string) ([]TaskStatus, error) {
	query := "SELECT id, last_run, start_time, duration FROM tasks"
	if taskID != nil {
		query += " WHERE id = ?"
	}
	query += " ORDER BY id"

	var rows *sql.Rows
	var err error
	if taskID != nil {
		rows, err = db.Query(query, *taskID)
	} else {
		rows, err = db.Query(query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []TaskStatus
	for rows.Next() {
		var id string
		var lastRun, startTime sql.NullString
		var duration sql.NullInt64
		if err := rows.Scan(&id, &lastRun, &startTime, &duration); err != nil {
			return nil, err
		}
		t := TaskStatus{ID: id}
		if lastRun.Valid {
			t.LastRun = format.ParseRFC3339Opt(lastRun.String)
		}
		if startTime.Valid {
			t.StartTime = format.ParseRFC3339Opt(startTime.String)
		}
		if duration.Valid {
			d := duration.Int64
			t.Duration = &d
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// CleanDB drops and recreates the `tasks` table, clearing every task while
// keeping log history intact.
func CleanDB(db *sql.DB) error {
	if _, err := db.Exec("DROP TABLE IF EXISTS tasks"); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE TABLE tasks (
		id TEXT PRIMARY KEY,
		last_run TEXT,
		start_time TEXT,
		duration INTEGER
	)`)
	return err
}

// DeleteTaskLogs removes all log entries for a task, returning the count.
func DeleteTaskLogs(db *sql.DB, taskID string) (int64, error) {
	res, err := db.Exec("DELETE FROM task_log WHERE id = ?", taskID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetTaskLogEntries returns log entries for a single task, retaining the raw
// end_time string. Ordered newest first.
func GetTaskLogEntries(db *sql.DB, taskID string) ([]LogEntry, error) {
	rows, err := db.Query(
		"SELECT end_time, elapsed_time FROM task_log WHERE id = ? ORDER BY end_time DESC",
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []LogEntry
	for rows.Next() {
		var endTimeStr string
		var elapsed int64
		if err := rows.Scan(&endTimeStr, &elapsed); err != nil {
			return nil, err
		}
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			return nil, err
		}
		entries = append(entries, LogEntry{Raw: endTimeStr, EndTime: endTime.UTC(), ElapsedMs: elapsed})
	}
	return entries, rows.Err()
}

// DeleteTaskLogEntry removes a single log entry identified by its end_time key.
func DeleteTaskLogEntry(db *sql.DB, taskID, endTimeStr string) (int64, error) {
	res, err := db.Exec("DELETE FROM task_log WHERE id = ? AND end_time = ?", taskID, endTimeStr)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteTask removes a task row, returning the count.
func DeleteTask(db *sql.DB, taskID string) (int64, error) {
	res, err := db.Exec("DELETE FROM tasks WHERE id = ?", taskID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CountOldLogs counts log entries older than cutoff, optionally for one task.
func CountOldLogs(db *sql.DB, cutoff time.Time, taskID *string) (int64, error) {
	cutoffStr := format.FormatRFC3339(cutoff)
	var count int64
	var err error
	if taskID != nil {
		err = db.QueryRow(
			"SELECT COUNT(*) FROM task_log WHERE end_time < ? AND id = ?",
			cutoffStr, *taskID,
		).Scan(&count)
	} else {
		err = db.QueryRow(
			"SELECT COUNT(*) FROM task_log WHERE end_time < ?",
			cutoffStr,
		).Scan(&count)
	}
	return count, err
}

// DeleteOldLogs deletes log entries older than cutoff, optionally for one task.
func DeleteOldLogs(db *sql.DB, cutoff time.Time, taskID *string) (int64, error) {
	cutoffStr := format.FormatRFC3339(cutoff)
	var res sql.Result
	var err error
	if taskID != nil {
		res, err = db.Exec(
			"DELETE FROM task_log WHERE end_time < ? AND id = ?",
			cutoffStr, *taskID,
		)
	} else {
		res, err = db.Exec("DELETE FROM task_log WHERE end_time < ?", cutoffStr)
	}
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// UpdateTaskDuration stores the most recent `check --duration` (in seconds).
func UpdateTaskDuration(db *sql.DB, id string, duration int64) error {
	_, err := db.Exec("UPDATE tasks SET duration = ? WHERE id = ?", duration, id)
	return err
}

// GetSetting returns a setting value and whether it was present.
func GetSetting(db *sql.DB, key string) (string, bool, error) {
	var value string
	err := db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	return value, true, nil
}

// SetSetting inserts or updates a setting value.
func SetSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

// GetAllSettings returns all settings ordered by key.
func GetAllSettings(db *sql.DB) ([]Setting, error) {
	rows, err := db.Query("SELECT key, value FROM settings ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []Setting
	for rows.Next() {
		var s Setting
		if err := rows.Scan(&s.Key, &s.Value); err != nil {
			return nil, err
		}
		settings = append(settings, s)
	}
	return settings, rows.Err()
}

// GetLogRetentionSeconds reads log_retention and returns it in seconds. The
// second return value is false when the setting is unset, "off", "0", or an
// invalid duration string.
func GetLogRetentionSeconds(db *sql.DB) (int64, bool, error) {
	value, ok, err := GetSetting(db, "log_retention")
	if err != nil {
		return 0, false, err
	}
	if !ok {
		return 0, false, nil
	}
	if strings.EqualFold(value, "off") || value == "0" {
		return 0, false, nil
	}
	d, err := format.ParseDuration(value)
	if err != nil {
		return 0, false, nil
	}
	return int64(d.Seconds()), true, nil
}

// SetLogRetention validates and stores the log_retention setting. "off"/"0"
// disables auto-cleanup; any other value must be a valid duration string.
func SetLogRetention(db *sql.DB, value string) error {
	if strings.EqualFold(value, "off") || value == "0" {
		return SetSetting(db, "log_retention", "off")
	}
	if _, err := format.ParseDuration(value); err != nil {
		return apperr.NewDurationParseError("Invalid duration, use e.g. 30d, 2w, 3m, 24h")
	}
	return SetSetting(db, "log_retention", value)
}
