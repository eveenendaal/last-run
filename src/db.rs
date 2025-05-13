use crate::error::{AppError, AppResult};
use chrono::{DateTime, Utc};
use dirs::home_dir;
use rusqlite::Connection;
use std::fs;

pub fn init_db(conn: &Connection) -> AppResult<()> {
    conn.execute(
        "CREATE TABLE IF NOT EXISTS tasks (
            id TEXT PRIMARY KEY,
            last_run TEXT,
            start_time TEXT,
            elapsed_time INTEGER
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

    Ok(())
}

pub fn get_file_based_connection() -> AppResult<Connection> {
    let home = home_dir().ok_or(AppError::HomeDirectoryNotFound)?;
    let db_dir = home.join(".tasks");

    fs::create_dir_all(&db_dir)?;

    let db_path = db_dir.join("data.db");
    let conn = Connection::open(db_path)?;

    Ok(conn)
}

/// Get all task logs from the database
pub fn get_task_logs(
    conn: &Connection,
    task_id: Option<String>,
    limit: usize,
) -> AppResult<Vec<(String, DateTime<Utc>, i64)>> {
    let mut query = String::from("SELECT id, end_time, elapsed_time FROM task_log");

    if let Some(_) = &task_id {
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
        let rows = stmt.query_map([id], map_log_row)?;
        for row in rows {
            logs.push(row?);
        }
    } else {
        let rows = stmt.query_map([], map_log_row)?;
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
    let mut query = String::from("SELECT id, last_run, start_time, elapsed_time FROM tasks");

    if let Some(_) = &task_id {
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
        let elapsed_time: Option<i64> = row.get(3)?;

        // Handle DateTime parsing safely to avoid error conversion issues
        let last_run = match last_run {
            Some(s) => match DateTime::parse_from_rfc3339(&s) {
                Ok(dt) => Some(dt.with_timezone(&Utc)),
                Err(_) => None,
            },
            None => None,
        };

        let start_time = match start_time {
            Some(s) => match DateTime::parse_from_rfc3339(&s) {
                Ok(dt) => Some(dt.with_timezone(&Utc)),
                Err(_) => None,
            },
            None => None,
        };

        Ok((id, last_run, start_time, elapsed_time))
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
