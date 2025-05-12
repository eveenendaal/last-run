use crate::error::AppResult;
use chrono::{DateTime, Utc};
use rusqlite::Connection;

pub struct Task {
    pub id: String,
    pub last_run: Option<DateTime<Utc>>,
    pub start_time: Option<DateTime<Utc>>, 
}

impl Task {
    pub fn new(id: String) -> Self {
        Task {
            id,
            last_run: None,
            start_time: None,
        }
    }

    pub fn update(&self, conn: &Connection) -> AppResult<()> {
        self.select(conn, false)?;

        // Update the last_run time if set
        if let Some(last_run) = self.last_run {
            conn.execute(
                "UPDATE tasks SET last_run = ? WHERE id = ?",
                (&last_run.to_rfc3339(), &self.id),
            )?;
        }

        // Update the start_time if set
        if let Some(start_time) = self.start_time {
            conn.execute(
                "UPDATE tasks SET start_time = ? WHERE id = ?",
                (&start_time.to_rfc3339(), &self.id),
            )?;
        }

        // Update the elapsed_time if start_time and last_run are set
        if let (Some(start_time), Some(last_run)) = (self.start_time, self.last_run) {
            let elapsed_time = last_run
                .signed_duration_since(start_time)
                .num_milliseconds();
            conn.execute(
                "UPDATE tasks SET elapsed_time = ? WHERE id = ?",
                (elapsed_time, &self.id),
            )?;

            // Insert a record into the log table
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

    pub fn select(&self, conn: &Connection, quiet: bool) -> AppResult<Option<Task>> {
        let mut stmt = conn.prepare("SELECT id, last_run, start_time FROM tasks WHERE id = ?")?;
        let mut rows = stmt.query([&self.id])?;

        if let Some(row) = rows.next()? {
            let id: String = row.get(0)?;
            let last_run_str: Option<String> = row.get(1)?;
            let last_run = last_run_str
                .map(|s| DateTime::parse_from_rfc3339(&s).ok())
                .flatten()
                .map(|dt| dt.with_timezone(&Utc));
            let start_time: Option<String> = row.get(2)?;
            let start_time = start_time
                .map(|s| DateTime::parse_from_rfc3339(&s).ok())
                .flatten()
                .map(|dt| dt.with_timezone(&Utc));

            Ok(Some(Task {
                id,
                last_run,
                start_time,
            }))
        } else {
            // No record found, insert a new one
            if !quiet {
                println!("No record found for task ID: {}", self.id);
            }
            self.insert(conn)?;
            Ok(Some(Task {
                id: self.id.clone(),
                last_run: self.last_run,
                start_time: self.start_time,
            }))
        }
    }

    pub fn start(&mut self, conn: &Connection) -> AppResult<()> {
        self.start_time = Some(Utc::now()); // Set the start time
        conn.execute(
            "UPDATE tasks SET start_time = ? WHERE id = ?",
            (&self.start_time.unwrap().to_rfc3339(), &self.id),
        )?;
        Ok(())
    }
}
