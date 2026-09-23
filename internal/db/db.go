// Package db owns the SQLite connection, schema, and all typed CRUD helpers.
// It uses the pure-Go modernc.org/sqlite driver so binaries are statically
// linked and cross-compile without cgo.
package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/eveenendaal/last-run/internal/apperr"
	"github.com/eveenendaal/last-run/internal/config"
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

// Task status values reported by TaskStatus.Status.
const (
	StatusOK      = "ok"
	StatusRunning = "running"
	StatusDue     = "due"
	StatusUnknown = "unknown"
)

// Status classifies the task at time now: "running" if started but not
// finished, "unknown" if it has never finished, "due" if its last run is older
// than its stored check duration, and "ok" otherwise.
func (t TaskStatus) Status(now time.Time) string {
	switch {
	case t.StartTime != nil && t.LastRun == nil:
		return StatusRunning
	case t.LastRun == nil:
		return StatusUnknown
	case t.Duration != nil && now.Sub(*t.LastRun) > time.Duration(*t.Duration)*time.Second:
		return StatusDue
	default:
		return StatusOK
	}
}

// Elapsed returns the run time of the task: the time since start for a running
// task, or start-to-finish for a completed one. ok is false when there is no
// meaningful value (never started, or the start is newer than the last run).
func (t TaskStatus) Elapsed(now time.Time) (d time.Duration, ok bool) {
	switch {
	case t.StartTime != nil && t.LastRun == nil:
		return now.Sub(*t.StartTime), true
	case t.StartTime != nil && t.StartTime.Before(*t.LastRun):
		return t.LastRun.Sub(*t.StartTime), true
	default:
		return 0, false
	}
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

// DefaultLogRetention is how long log entries are kept when no log_retention
// setting has been stored.
const DefaultLogRetention = 30 * 24 * time.Hour

const createTasksTable = `CREATE TABLE IF NOT EXISTS tasks (
	id TEXT PRIMARY KEY,
	last_run TEXT,
	start_time TEXT,
	duration INTEGER
)`

// InitDB creates the schema idempotently and ensures the `duration` column
// exists on older databases.
func InitDB(db *sql.DB) error {
	stmts := []string{
		createTasksTable,
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

// Open opens (or creates) a SQLite database at the given path, creating the
// parent directory if needed. The connection pool is capped at one to avoid
// "database is locked" contention from within the same process. A 5-second
// busy timeout and WAL journal mode handle cross-process contention gracefully.
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	for _, pragma := range []string{"PRAGMA busy_timeout = 5000", "PRAGMA journal_mode = WAL"} {
		if _, err := database.Exec(pragma); err != nil {
			_ = database.Close()
			return nil, err
		}
	}
	return database, nil
}

// ResolveDBPath returns the database path to use, following this priority:
// 1. override (CLI flag / LASTRUN_DB_PATH env var)
// 2. db_path from $XDG_CONFIG_HOME/lastrun/config.json
// 3. XDG data-home default (~/.local/share/lastrun/data.db on Linux)
func ResolveDBPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if cfg.DBPath != "" {
		return cfg.DBPath, nil
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

// CopyDatabase copies the currently-open srcDB to dstPath using SQLite's
// VACUUM INTO, which creates a clean, defragmented copy. The destination
// directory is created if needed. Returns an error if dstPath already exists;
// remove the file first if you need to overwrite.
func CopyDatabase(srcDB *sql.DB, dstPath string) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dstPath); err == nil {
		return fmt.Errorf("destination already exists: %s", dstPath)
	}
	_, err := srcDB.Exec("VACUUM INTO ?", dstPath)
	return err
}

// GetCustomDBPath returns the db_path stored in the user config file, or an
// empty string when no custom path has been set.
func GetCustomDBPath() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return cfg.DBPath, nil
}

// SetCustomDBPath writes path to the user config file as the preferred DB
// location. Pass an empty string to clear the override and revert to the
// XDG-based default.
func SetCustomDBPath(path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.DBPath = path
	return config.Save(cfg)
}

// GetTaskLogs returns recent log entries, optionally filtered by task ID. A
// limit of 0 means no limit. Entries are ordered newest first.
func GetTaskLogs(db *sql.DB, taskID *string, limit int) ([]LogRow, error) {
	where, args := idFilter("WHERE", taskID)
	query := "SELECT id, end_time, elapsed_time FROM task_log" + where + " ORDER BY end_time DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.Query(query, args...)
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
			return nil, fmt.Errorf("date parse error: %w", err)
		}
		logs = append(logs, LogRow{ID: id, EndTime: endTime.UTC(), ElapsedMs: elapsed})
	}
	return logs, rows.Err()
}

// GetAllTasks returns all tasks (optionally filtered by ID), ordered by ID.
func GetAllTasks(db *sql.DB, taskID *string) ([]TaskStatus, error) {
	where, args := idFilter("WHERE", taskID)
	rows, err := db.Query("SELECT id, last_run, start_time, duration FROM tasks"+where+" ORDER BY id", args...)
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
		t := TaskStatus{
			ID:        id,
			LastRun:   format.ParseRFC3339Opt(lastRun.String),
			StartTime: format.ParseRFC3339Opt(startTime.String),
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
	_, err := db.Exec(createTasksTable)
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
	where, args := olderThan(cutoff, taskID)
	var count int64
	err := db.QueryRow("SELECT COUNT(*) FROM task_log"+where, args...).Scan(&count)
	return count, err
}

// DeleteOldLogs deletes log entries older than cutoff, optionally for one task.
func DeleteOldLogs(db *sql.DB, cutoff time.Time, taskID *string) (int64, error) {
	where, args := olderThan(cutoff, taskID)
	res, err := db.Exec("DELETE FROM task_log"+where, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// idFilter returns an " <keyword> id = ?" clause and its argument when taskID
// is set, or nothing when it is nil.
func idFilter(keyword string, taskID *string) (string, []any) {
	if taskID == nil {
		return "", nil
	}
	return " " + keyword + " id = ?", []any{*taskID}
}

// olderThan builds the WHERE clause selecting log entries that ended before
// cutoff, optionally restricted to one task.
func olderThan(cutoff time.Time, taskID *string) (string, []any) {
	and, idArgs := idFilter("AND", taskID)
	return " WHERE end_time < ?" + and, append([]any{format.FormatRFC3339(cutoff)}, idArgs...)
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
		if errors.Is(err, sql.ErrNoRows) {
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

// LogRetention returns the effective log retention period. It is
// DefaultLogRetention when the setting is unset or invalid, and 0 when
// auto-cleanup has been turned off ("off" or "0").
func LogRetention(db *sql.DB) (time.Duration, error) {
	value, ok, err := GetSetting(db, logRetentionKey)
	if err != nil || !ok {
		return DefaultLogRetention, err
	}
	if IsRetentionOff(value) {
		return 0, nil
	}
	d, err := format.ParseDuration(value)
	if err != nil {
		return DefaultLogRetention, nil
	}
	return d, nil
}

// SetLogRetention validates and stores the log_retention setting. "off"/"0"
// disables auto-cleanup; any other value must be a valid duration string.
func SetLogRetention(db *sql.DB, value string) error {
	if IsRetentionOff(value) {
		return SetSetting(db, logRetentionKey, "off")
	}
	if _, err := format.ParseDuration(value); err != nil {
		return apperr.NewDurationParseError("Invalid duration, use e.g. 30d, 2w, 3m, 24h")
	}
	return SetSetting(db, logRetentionKey, value)
}

const logRetentionKey = "log_retention"

// IsRetentionOff reports whether value disables log auto-cleanup ("off" or "0").
func IsRetentionOff(value string) bool {
	return strings.EqualFold(value, "off") || value == "0"
}
