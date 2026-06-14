use crate::error::{AppError, AppResult};
use crate::format::{parse_duration, parse_rfc3339_opt};
use chrono::{DateTime, Utc};
use dirs::{data_dir, home_dir};
use rusqlite::Connection;
use std::fs;
use std::path::{Path, PathBuf};

pub fn init_db(conn: &Connection) -> AppResult<()> {
    conn.execute(
        "CREATE TABLE IF NOT EXISTS tasks (
            id TEXT PRIMARY KEY,
            last_run TEXT,
            start_time TEXT,
            duration INTEGER
        )",
        [],
    )?;

    conn.execute(
        "CREATE TABLE IF NOT EXISTS task_log (
            id TEXT,
            end_time TEXT,
            elapsed_time INTEGER,
            PRIMARY KEY (id, end_time)
        )",
        [],
    )?;

    conn.execute(
        "CREATE TABLE IF NOT EXISTS settings (
            key TEXT PRIMARY KEY,
            value TEXT NOT NULL
        )",
        [],
    )?;

    // Ensure the `duration` column exists in the `tasks` table
    conn.execute("ALTER TABLE tasks ADD COLUMN duration INTEGER", [])
        .ok();

    Ok(())
}

pub fn get_file_based_connection(db_path_override: Option<PathBuf>) -> AppResult<Connection> {
    let is_default = db_path_override.is_none();
    let db_path = resolve_db_path(db_path_override)?;

    if is_default {
        maybe_migrate_old_data(&db_path).ok();
    }

    if let Some(parent) = db_path.parent() {
        fs::create_dir_all(parent)?;
    }

    let conn = Connection::open(&db_path)?;
    Ok(conn)
}

fn resolve_db_path(db_path_override: Option<PathBuf>) -> AppResult<PathBuf> {
    match db_path_override {
        Some(path) => Ok(path),
        None => default_db_path(),
    }
}

fn default_db_path() -> AppResult<PathBuf> {
    let data_dir = data_dir().ok_or(AppError::DataDirectoryNotFound)?;
    Ok(data_dir.join("lastrun").join("data.db"))
}

fn maybe_migrate_old_data(new_path: &Path) -> AppResult<()> {
    let old_path = home_dir().map(|h| h.join(".tasks").join("data.db"));

    if let Some(old_path) = old_path {
        if old_path.exists() && !new_path.exists() {
            if let Some(parent) = new_path.parent() {
                fs::create_dir_all(parent)?;
            }
            fs::rename(&old_path, new_path)?;
            if let Some(old_dir) = old_path.parent() {
                let _ = fs::remove_dir(old_dir);
            }
        } else if old_path.exists() && new_path.exists() {
            eprintln!(
                "Warning: old database at {} and new database at {} both exist.\n  Using new location. Remove the old file manually:\n  rm {}",
                old_path.display(),
                new_path.display(),
                old_path.display()
            );
        }
    }

    Ok(())
}

/// Get all task logs from the database
pub fn get_task_logs(
    conn: &Connection,
    task_id: Option<String>,
    limit: usize,
) -> AppResult<Vec<(String, DateTime<Utc>, i64)>> {
    let mut query = String::from("SELECT id, end_time, elapsed_time FROM task_log");

    if task_id.is_some() {
        query.push_str(" WHERE id = ?");
    }

    query.push_str(" ORDER BY end_time DESC");

    if limit > 0 {
        query.push_str(&format!(" LIMIT {}", limit));
    }

    let mut stmt = conn.prepare(&query)?;

    // Create a mapping function for the rows to handle parsing correctly
    let map_log_row = |row: &rusqlite::Row| -> rusqlite::Result<(String, DateTime<Utc>, i64)> {
        let id: String = row.get(0)?;
        let end_time_str: String = row.get(1)?;
        let elapsed_time: i64 = row.get(2)?;

        // Handle DateTime parsing outside the ? operator to avoid type conversion issues
        let end_time = match DateTime::parse_from_rfc3339(&end_time_str) {
            Ok(dt) => dt.with_timezone(&Utc),
            Err(err) => {
                return Err(rusqlite::Error::InvalidParameterName(format!(
                    "Date parse error: {}",
                    err
                )))
            }
        };

        Ok((id, end_time, elapsed_time))
    };

    let mut logs = Vec::new();

    // Use the mapping function with params
    if let Some(id) = task_id {
        let rows = stmt.query_map([id], map_log_row)?; // Pass task ID as parameter
        for row in rows {
            logs.push(row?);
        }
    } else {
        let rows = stmt.query_map([], map_log_row)?; // No filtering
        for row in rows {
            logs.push(row?);
        }
    }

    Ok(logs)
}

/// Get all tasks with their current status
pub fn get_all_tasks(
    conn: &Connection,
    task_id: Option<String>,
) -> AppResult<
    Vec<(
        String,
        Option<DateTime<Utc>>,
        Option<DateTime<Utc>>,
        Option<i64>,
    )>,
> {
    let mut query = String::from("SELECT id, last_run, start_time, duration FROM tasks");

    if task_id.is_some() {
        query.push_str(" WHERE id = ?");
    }

    query.push_str(" ORDER BY id");

    let mut stmt = conn.prepare(&query)?;

    // Create a mapping function for the rows to handle parsing correctly
    let map_task_row = |row: &rusqlite::Row| -> rusqlite::Result<(
        String,
        Option<DateTime<Utc>>,
        Option<DateTime<Utc>>,
        Option<i64>,
    )> {
        let id: String = row.get(0)?;
        let last_run: Option<String> = row.get(1)?;
        let start_time: Option<String> = row.get(2)?;
        let duration: Option<i64> = row.get(3)?;

        let last_run = parse_rfc3339_opt(last_run);
        let start_time = parse_rfc3339_opt(start_time);

        Ok((id, last_run, start_time, duration))
    };

    let mut tasks = Vec::new();

    // Use the mapping function with params
    if let Some(id) = task_id {
        let rows = stmt.query_map([id], map_task_row)?;
        for row in rows {
            tasks.push(row?);
        }
    } else {
        let rows = stmt.query_map([], map_task_row)?;
        for row in rows {
            tasks.push(row?);
        }
    }

    Ok(tasks)
}

pub fn clean_db(conn: &Connection) -> AppResult<()> {
    conn.execute("DROP TABLE IF EXISTS tasks", [])?;
    conn.execute(
        "CREATE TABLE tasks (
            id TEXT PRIMARY KEY,
            last_run TEXT,
            start_time TEXT,
            duration INTEGER
        )",
        [],
    )?;
    Ok(())
}

/// Delete task logs for a specific task
pub fn delete_task_logs(conn: &Connection, task_id: &str) -> AppResult<usize> {
    let rows_affected = conn.execute("DELETE FROM task_log WHERE id = ?", [task_id])?;

    Ok(rows_affected)
}

/// Get log entries for a single task, returning the raw end_time string alongside parsed values.
/// Returns Vec<(raw_end_time, end_time, elapsed_ms)> ordered newest first.
pub fn get_task_log_entries(
    conn: &Connection,
    task_id: &str,
) -> AppResult<Vec<(String, DateTime<Utc>, i64)>> {
    let mut stmt = conn.prepare(
        "SELECT end_time, elapsed_time FROM task_log WHERE id = ? ORDER BY end_time DESC",
    )?;

    let raw_rows: Vec<(String, i64)> = stmt
        .query_map([task_id], |row| {
            Ok((row.get::<_, String>(0)?, row.get::<_, i64>(1)?))
        })?
        .collect::<rusqlite::Result<Vec<_>>>()?;

    raw_rows
        .into_iter()
        .map(|(end_time_str, elapsed_time)| {
            let end_time = DateTime::parse_from_rfc3339(&end_time_str)
                .map_err(crate::error::AppError::DateParse)?
                .with_timezone(&Utc);
            Ok((end_time_str, end_time, elapsed_time))
        })
        .collect()
}

/// Delete a single log entry for a task by its end_time key
pub fn delete_task_log_entry(conn: &Connection, task_id: &str, end_time_str: &str) -> AppResult<usize> {
    let rows_affected = conn.execute(
        "DELETE FROM task_log WHERE id = ? AND end_time = ?",
        rusqlite::params![task_id, end_time_str],
    )?;
    Ok(rows_affected)
}

/// Delete a task record
pub fn delete_task(conn: &Connection, task_id: &str) -> AppResult<usize> {
    let rows_affected = conn.execute("DELETE FROM tasks WHERE id = ?", [task_id])?;

    Ok(rows_affected)
}

/// Count log entries older than the given cutoff timestamp, optionally filtered by task ID
pub fn count_old_logs(
    conn: &Connection,
    cutoff: &DateTime<Utc>,
    task_id: Option<&str>,
) -> AppResult<usize> {
    let cutoff_str = cutoff.to_rfc3339();
    let count: i64 = match task_id {
        Some(id) => conn.query_row(
            "SELECT COUNT(*) FROM task_log WHERE end_time < ? AND id = ?",
            rusqlite::params![cutoff_str, id],
            |row| row.get(0),
        )?,
        None => conn.query_row(
            "SELECT COUNT(*) FROM task_log WHERE end_time < ?",
            rusqlite::params![cutoff_str],
            |row| row.get(0),
        )?,
    };
    Ok(count as usize)
}

/// Delete log entries older than the given cutoff timestamp, optionally filtered by task ID
pub fn delete_old_logs(
    conn: &Connection,
    cutoff: &DateTime<Utc>,
    task_id: Option<&str>,
) -> AppResult<usize> {
    let cutoff_str = cutoff.to_rfc3339();
    let rows_affected = match task_id {
        Some(id) => conn.execute(
            "DELETE FROM task_log WHERE end_time < ? AND id = ?",
            rusqlite::params![cutoff_str, id],
        )?,
        None => conn.execute(
            "DELETE FROM task_log WHERE end_time < ?",
            rusqlite::params![cutoff_str],
        )?,
    };
    Ok(rows_affected)
}

pub fn update_task_duration(conn: &Connection, id: &str, duration: i64) -> AppResult<()> {
    conn.execute(
        "UPDATE tasks SET duration = ? WHERE id = ?",
        rusqlite::params![duration, id], // Use `rusqlite::params!` to handle mixed types
    )?;
    Ok(())
}

/// Get a setting value by key
pub fn get_setting(conn: &Connection, key: &str) -> AppResult<Option<String>> {
    let mut stmt = conn.prepare("SELECT value FROM settings WHERE key = ?")?;
    let mut rows = stmt.query_map([key], |row| row.get::<_, String>(0))?;
    match rows.next() {
        Some(Ok(value)) => Ok(Some(value)),
        _ => Ok(None),
    }
}

/// Set a setting value (insert or update)
pub fn set_setting(conn: &Connection, key: &str, value: &str) -> AppResult<()> {
    conn.execute(
        "INSERT INTO settings (key, value) VALUES (?, ?)
         ON CONFLICT(key) DO UPDATE SET value = excluded.value",
        rusqlite::params![key, value],
    )?;
    Ok(())
}

/// Get all settings as key-value pairs
pub fn get_all_settings(conn: &Connection) -> AppResult<Vec<(String, String)>> {
    let mut stmt = conn.prepare("SELECT key, value FROM settings ORDER BY key")?;
    let rows = stmt.query_map([], |row| {
        Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?))
    })?;
    let mut settings = Vec::new();
    for row in rows {
        settings.push(row?);
    }
    Ok(settings)
}

/// Read the log_retention setting and return it in seconds.
/// Returns None if the setting is unset, "off", "0", or an invalid duration string.
pub fn get_log_retention_seconds(conn: &Connection) -> AppResult<Option<i64>> {
    let value = match get_setting(conn, "log_retention")? {
        Some(v) => v,
        None => return Ok(None),
    };
    if value.eq_ignore_ascii_case("off") || value == "0" {
        return Ok(None);
    }
    match parse_duration(&value) {
        Ok(d) => Ok(Some(d.num_seconds())),
        Err(_) => Ok(None),
    }
}

/// Parse and store the log_retention setting. Accepts a duration string like "30d" or "off" to disable.
pub fn set_log_retention(conn: &Connection, value: &str) -> AppResult<()> {
    if value.eq_ignore_ascii_case("off") || value == "0" {
        set_setting(conn, "log_retention", "off")
    } else {
        // Validate by parsing
        parse_duration(value).map_err(|_| AppError::DurationParse("Invalid duration, use e.g. 30d, 2w, 3m, 24h".to_string()))?;
        set_setting(conn, "log_retention", value)
    }
}
