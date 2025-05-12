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
    last_run: Option<DateTime<Utc>>,
    start_time: Option<DateTime<Utc>>, // Added start_time field
}

impl Task {
    fn new(id: String) -> Self {
        Task {
            id,
            last_run: None,
            start_time: None,
        }
    }

    fn update(&self, conn: &Connection) -> AppResult<()> {
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

    fn insert(&self, conn: &Connection) -> AppResult<()> {
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

    fn select(&self, conn: &Connection, quiet: bool) -> AppResult<Option<Task>> {
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

    fn start(&mut self, conn: &Connection) -> AppResult<()> {
        self.start_time = Some(Utc::now()); // Set the start time
        conn.execute(
            "UPDATE tasks SET start_time = ? WHERE id = ?",
            (&self.start_time.unwrap().to_rfc3339(), &self.id),
        )?;
        Ok(())
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

fn format_duration_hundredths(duration: Duration) -> String {
    let total_milliseconds = duration.num_milliseconds();
    let seconds = total_milliseconds / 1000;
    let hundredths = (total_milliseconds % 1000) / 10;

    format!("{}.{}s", seconds, hundredths)
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
    /// Suppress output messages
    #[arg(short, long)]
    quiet: bool,

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

    /// Synonym for update
    Done {
        /// Task ID to mark as done
        #[arg(short, long)]
        id: String,
    },

    /// Start a task
    Start {
        /// Task ID to start
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

const BOLD: &str = "\x1b[1m";
const RESET: &str = "\x1b[0m";
const GREEN: &str = "\x1b[32m";
const RED: &str = "\x1b[31m";
const WHITE: &str = "\x1b[97m"; // Updated to brighter white

fn main() -> AppResult<()> {
    let cli = Cli::parse();

    let conn = init_db()?;

    match cli.command {
        Commands::Update { id } | Commands::Done { id } => {
            if id.is_empty() {
                return Err(AppError::MissingTaskId);
            }

            let mut task = Task::new(id);
            task.last_run = Some(Utc::now());
            let mut elapsed_time: Option<String> = None;

            if let Some(existing_task) = task.select(&conn, cli.quiet)? {
                task.start_time = existing_task.start_time; // Preserve the existing start_time
                if let Some(start_time) = task.start_time {
                    let elapsed = Utc::now().signed_duration_since(start_time);
                    elapsed_time = Some(format_duration_hundredths(elapsed));
                }
                task.update(&conn)?;
            }

            if !cli.quiet {
                let elapsed_msg = elapsed_time
                    .map(|elapsed| format!(". Elapsed time: {}{}{}", WHITE, elapsed, GREEN))
                    .unwrap_or_default();

                println!(
                    "{}{}Task {}{}{} finished at {}{}{}{}",
                    BOLD,
                    GREEN,
                    WHITE,
                    task.id,
                    GREEN,
                    WHITE,
                    task.last_run.unwrap().to_rfc3339(),
                    elapsed_msg,
                    RESET
                );
            }
        }

        Commands::Start { id } => {
            if id.is_empty() {
                return Err(AppError::MissingTaskId);
            }

            let mut task = Task::new(id);
            task.start_time = Some(Utc::now());
            if let Some(existing_task) = task.select(&conn, cli.quiet)? {
                task.start_time = existing_task.start_time; // Preserve the existing start time if any
            }
            task.start(&conn)?;

            if !cli.quiet {
                println!(
                    "{}{}Task {}{}{} started at {}{}{}",
                    BOLD,
                    GREEN,
                    WHITE,
                    task.id,
                    GREEN,
                    WHITE,
                    task.start_time.unwrap().to_rfc3339(),
                    RESET
                );
            }
        }

        Commands::Check { id, duration } => {
            if id.is_empty() {
                return Err(AppError::MissingTaskId);
            }

            let duration = parse_duration(&duration)?;

            let task = Task::new(id);
            if let Some(existing_task) = task.select(&conn, cli.quiet)? {
                if let Some(last_run) = existing_task.last_run {
                    let (should_run, message) = should_run_task(last_run, duration);
                    if !cli.quiet {
                        println!(
                            "{}{}{}{}",
                            BOLD,
                            if should_run { RED } else { GREEN },
                            message,
                            RESET
                        );
                    }

                    if should_run {
                        process::exit(1);
                    }
                } else {
                    if !cli.quiet {
                        println!(
                            "{}{}Task {}{}{} has no recorded last run. It is considered due.{}",
                            BOLD, RED, WHITE, task.id, RED, RESET
                        );
                    }
                    process::exit(1);
                }
            } else {
                if !cli.quiet {
                    println!(
                        "{}{}No record found for task ID: {}{}{}",
                        BOLD, RED, WHITE, task.id, RESET
                    );
                }
            }
        }
    }

    Ok(())
}
