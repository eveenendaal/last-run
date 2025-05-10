use chrono::{DateTime, Duration, Utc};
use clap::{Parser, Subcommand};
use dirs::home_dir;
use rusqlite::Connection;
use std::fs;
use std::process;
use thiserror::Error;

#[derive(Error, Debug)]
enum AppError {
    #[error("Database error: {0}")]
    Database(#[from] rusqlite::Error),

    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),

    #[error("Date parsing error: {0}")]
    DateParse(#[from] chrono::ParseError),

    #[error("Duration parsing error: {0}")]
    DurationParse(String),

    #[error("Task ID is required")]
    MissingTaskId,

    #[error("Home directory not found")]
    HomeDirectoryNotFound,
}

type AppResult<T> = Result<T, AppError>;

struct Task {
    id: String,
    last_run: DateTime<Utc>,
}

impl Task {
    fn new(id: String, last_run: DateTime<Utc>) -> Self {
        Task { id, last_run }
    }

    fn update(&self, conn: &Connection) -> AppResult<()> {
        self.select(conn)?;

        conn.execute(
            "UPDATE tasks SET last_run = ? WHERE id = ?",
            (&self.last_run.to_rfc3339(), &self.id),
        )?;

        Ok(())
    }

    fn insert(&self, conn: &Connection) -> AppResult<()> {
        conn.execute(
            "INSERT INTO tasks (id, last_run) VALUES (?, ?)",
            (&self.id, &self.last_run.to_rfc3339()),
        )?;

        Ok(())
    }

    fn select(&self, conn: &Connection) -> AppResult<Option<Task>> {
        let mut stmt = conn.prepare("SELECT id, last_run FROM tasks WHERE id = ?")?;
        let mut rows = stmt.query([&self.id])?;

        if let Some(row) = rows.next()? {
            let id: String = row.get(0)?;
            let last_run_str: String = row.get(1)?;
            let last_run = DateTime::parse_from_rfc3339(&last_run_str)?.with_timezone(&Utc);

            Ok(Some(Task { id, last_run }))
        } else {
            // No record found, insert a new one
            println!("No record found for task ID: {}", self.id);
            self.insert(conn)?;
            Ok(Some(Task {
                id: self.id.clone(),
                last_run: self.last_run,
            }))
        }
    }
}

fn init_db() -> AppResult<Connection> {
    let home = home_dir().ok_or(AppError::HomeDirectoryNotFound)?;
    let db_dir = home.join(".tasks");

    fs::create_dir_all(&db_dir)?;

    let db_path = db_dir.join("data.db");
    let conn = Connection::open(db_path)?;

    conn.execute(
        "CREATE TABLE IF NOT EXISTS tasks (
            id TEXT PRIMARY KEY,
            last_run TEXT
        )",
        [],
    )?;

    Ok(conn)
}

fn parse_duration(duration_str: &str) -> AppResult<Duration> {
    if let Some(hours_str) = duration_str.strip_suffix('h') {
        if let Ok(hours) = hours_str.parse::<i64>() {
            return Ok(Duration::hours(hours));
        }
    } else if let Some(days_str) = duration_str.strip_suffix('d') {
        if let Ok(days) = days_str.parse::<i64>() {
            return Ok(Duration::days(days));
        }
    }

    Err(AppError::DurationParse(format!(
        "Invalid duration format: {}",
        duration_str
    )))
}

fn format_duration(duration: Duration) -> String {
    let total_minutes = duration.num_minutes();
    let days = total_minutes / (24 * 60);
    let hours = (total_minutes % (24 * 60)) / 60;
    let minutes = total_minutes % 60;

    if days > 0 {
        format!("{}d{}h{}m", days, hours, minutes)
    } else if hours > 0 {
        format!("{}h{}m", hours, minutes)
    } else {
        format!("{}m", minutes)
    }
}

fn should_run_task(last_run: DateTime<Utc>, duration: Duration) -> (bool, String) {
    let time_since_last_run = Utc::now().signed_duration_since(last_run);

    if time_since_last_run >= duration {
        (
            true,
            format!(
                "Task is due (last run: {}, {} ago)",
                last_run.to_rfc3339(),
                format_duration(time_since_last_run)
            ),
        )
    } else {
        (
            false,
            format!(
                "Task is not due yet (last run: {}, {} ago, threshold: {})",
                last_run.to_rfc3339(),
                format_duration(time_since_last_run),
                format_duration(duration)
            ),
        )
    }
}

#[derive(Parser)]
#[command(name = "lastrun")]
#[command(about = "A utility to track when tasks were last run")]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Update a task's last run time
    Update {
        /// Task ID to update
        #[arg(short, long)]
        id: String,
    },

    /// Check if a task is due to run
    Check {
        /// Task ID to check
        #[arg(short, long)]
        id: String,

        /// Duration threshold (e.g., 24h, 7d)
        #[arg(short, long, default_value = "24h")]
        duration: String,
    },
}

fn main() -> AppResult<()> {
    let cli = Cli::parse();

    let conn = init_db()?;

    match cli.command {
        Commands::Update { id } => {
            if id.is_empty() {
                return Err(AppError::MissingTaskId);
            }

            let task = Task::new(id, Utc::now());
            task.update(&conn)?;

            println!("Task {} updated at {}", task.id, task.last_run.to_rfc3339());
        }

        Commands::Check { id, duration } => {
            if id.is_empty() {
                return Err(AppError::MissingTaskId);
            }

            let duration = parse_duration(&duration)?;

            let task = Task::new(id, DateTime::<Utc>::from(std::time::UNIX_EPOCH));
            if let Some(existing_task) = task.select(&conn)? {
                let (should_run, message) = should_run_task(existing_task.last_run, duration);
                println!("{}", message);

                if should_run {
                    process::exit(1);
                }
            }
        }
    }

    Ok(())
}
