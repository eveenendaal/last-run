use crate::error::AppResult;
use crate::format::parse_rfc3339_opt;
use chrono::{DateTime, Utc};
use rusqlite::{Connection, Row};

pub struct Task {
    pub id: String,
    pub last_run: Option<DateTime<Utc>>,
    pub start_time: Option<DateTime<Utc>>,
}

impl Task {
    fn from_row(row: &Row) -> rusqlite::Result<Self> {
        let id: String = row.get(0)?;
        let last_run = parse_rfc3339_opt(row.get(1)?);
        let start_time = parse_rfc3339_opt(row.get(2)?);
        Ok(Task {
            id,
            last_run,
            start_time,
        })
    }

    pub fn update(&self, conn: &Connection) -> AppResult<()> {
        // Combine updates into a single statement for efficiency
        conn.execute(
            "UPDATE tasks SET last_run = ?, start_time = ? WHERE id = ?",
            (
                &self.last_run.map(|dt| dt.to_rfc3339()),
                &self.start_time.map(|dt| dt.to_rfc3339()),
                &self.id,
            ),
        )?;

        // Insert a record into the log table if start_time and last_run are set
        if let (Some(start_time), Some(last_run)) = (self.start_time, self.last_run) {
            let elapsed_time = last_run
                .signed_duration_since(start_time)
                .num_milliseconds();
            conn.execute(
                "INSERT INTO task_log (id, end_time, elapsed_time) VALUES (?, ?, ?)",
                (&self.id, &last_run.to_rfc3339(), elapsed_time),
            )?;
        }

        Ok(())
    }

    pub fn insert(&self, conn: &Connection) -> AppResult<()> {
        conn.execute(
            "INSERT INTO tasks (id, last_run, start_time) VALUES (?, ?, ?)",
            (
                &self.id,
                &self.last_run.map(|dt| dt.to_rfc3339()),
                &self.start_time.map(|dt| dt.to_rfc3339()),
            ),
        )?;

        Ok(())
    }

    pub fn select(conn: &Connection, id: &str) -> AppResult<Option<Self>> {
        let mut stmt = conn.prepare("SELECT id, last_run, start_time FROM tasks WHERE id = ?")?;
        let mut rows = stmt.query([id])?;

        if let Some(row) = rows.next()? {
            Ok(Some(Task::from_row(&row)?))
        } else {
            Ok(None)
        }
    }

    pub fn ensure(conn: &Connection, id: &str, quiet: bool) -> AppResult<Self> {
        match Self::select(conn, id)? {
            Some(task) => Ok(task),
            None => {
                if !quiet {
                    println!("No record found for task ID: {}", id);
                }
                let task = Task {
                    id: id.to_string(),
                    last_run: None,
                    start_time: None,
                };
                task.insert(conn)?;
                Ok(task)
            }
        }
    }
}
