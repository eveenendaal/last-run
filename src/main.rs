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

    /// Display execution logs for tasks
    Logs {
        /// Limit number of logs to show (0 for all)
        #[arg(short, long, default_value = "20")]
        limit: usize,

        /// Filter logs by task ID
        #[arg(short, long)]
        id: Option<String>,
    },

    /// Display current status of all tasks
    Status {
        /// Filter tasks by ID
        #[arg(short, long)]
        id: Option<String>,
    },
}

const BOLD: &str = "\x1b[1m";
const RESET: &str = "\x1b[0m";
const GREEN: &str = "\x1b[32m";
const RED: &str = "\x1b[31m";
const WHITE: &str = "\x1b[97m"; // Updated to brighter white
const BLUE: &str = "\x1b[34m";
const YELLOW: &str = "\x1b[33m";
const CYAN: &str = "\x1b[36m";

/// Get all task logs from the database
fn get_task_logs(
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
fn get_all_tasks(
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

/// Format and print task status
fn print_task_status(
    tasks: &[(
        String,
        Option<DateTime<Utc>>,
        Option<DateTime<Utc>>,
        Option<i64>,
    )],
) {
    // Calculate dynamic column widths
    let mut id_width = 12; // Minimum width
    let last_run_width = 19;
    let started_width = 19;
    let elapsed_width = 8;

    // Find the maximum ID length
    for (id, _, _, _) in tasks {
        id_width = id_width.max(id.len());
    }
    id_width += 2;

    // Calculate total width
    let total_width = id_width + last_run_width + started_width + elapsed_width + 5; // 5 for borders

    // Top border
    println!(
        "\n{}{}╔{}╗{}",
        BOLD, CYAN,
        "═".repeat(total_width - 2),
        RESET
    );

    // Title
    let title = "TASK STATUS";
    let padding_total = total_width - 2 - title.len();
    let padding_left = padding_total / 2;
    let padding_right = padding_total - padding_left;
    println!(
        "{}{}║{}{}{}║{}",
        BOLD, CYAN,
        " ".repeat(padding_left), title, " ".repeat(padding_right),
        RESET
    );

    // Header border
    println!(
        "{}{}╠{}╦{}╦{}╦{}╣{}",
        BOLD, CYAN,
        "═".repeat(id_width),
        "═".repeat(last_run_width),
        "═".repeat(started_width),
        "═".repeat(elapsed_width),
        RESET
    );

    if tasks.is_empty() {
        println!(
            "{bold}{cyan}║{msg:<width$}║{reset}",
            bold = BOLD,
            cyan = CYAN,
            msg = " No tasks found",
            width = total_width - 4,
            reset = RESET
        );
    } else {
        // Column headers
        println!(
            "{bold}{cyan}║{id:<idw$}║{last:<lrw$}║{start:<stw$}║{elapsed:<elw$}║{reset}",
            bold = BOLD,
            cyan = CYAN,
            id = "TASK ID",
            idw = id_width,
            last = "LAST RUN",
            lrw = last_run_width,
            start = "STARTED",
            stw = started_width,
            elapsed = "ELAPSED",
            elw = elapsed_width,
            reset = RESET
        );

        // Header/content separator
        println!(
            "{}{}╠{}╬{}╬{}╬{}╣{}",
            BOLD, CYAN,
            "═".repeat(id_width),
            "═".repeat(last_run_width),
            "═".repeat(started_width),
            "═".repeat(elapsed_width),
            RESET
        );

        for (id, last_run, start_time, elapsed_time) in tasks {
            let now = Utc::now();
            let status_color = if start_time.is_some() && last_run.is_none() {
                YELLOW
            } else if let Some(lr) = last_run {
                if now.signed_duration_since(*lr) > Duration::days(1) {
                    RED
                } else {
                    GREEN
                }
            } else {
                BLUE
            };

            let last_run_str = if let Some(lr) = last_run {
                format!("{}", lr.format("%Y-%m-%d %H:%M:%S"))
            } else {
                "never".to_string()
            };

            let start_time_str = if let Some(st) = start_time {
                format!("{}", st.format("%Y-%m-%d %H:%M:%S"))
            } else {
                "-".to_string()
            };

            let elapsed_str = if let Some(et) = elapsed_time {
                format_duration_hundredths(Duration::milliseconds(*et))
            } else {
                "-".to_string()
            };

            println!(
                "{bold}{cyan}║{color}{id:<idw$}{cyan}║{color}{last:<lrw$}{cyan}║{color}{start:<stw$}{cyan}║{color}{elapsed:<elw$}{cyan}║{reset}",
                bold = BOLD,
                cyan = CYAN,
                color = status_color,
                id = id,
                idw = id_width,
                last = last_run_str,
                lrw = last_run_width,
                start = start_time_str,
                stw = started_width,
                elapsed = elapsed_str,
                elw = elapsed_width,
                reset = RESET
            );
        }
    }

    // Bottom border
    println!(
        "{}{}╚{}╝{}",
        BOLD, CYAN,
        "═".repeat(total_width - 2),
        RESET
    );
}

/// Format and print task logs
fn print_task_logs(logs: &[(String, DateTime<Utc>, i64)]) {
    // Calculate dynamic column widths
    let mut id_width = 12; // Minimum width
    let completion_width = 19;
    let duration_width = 8;

    // Find the maximum ID length
    for (id, _, _) in logs {
        id_width = id_width.max(id.len());
    }
    id_width += 2;

    // Calculate total width
    let total_width = id_width + completion_width + duration_width + 4; // 4 for borders

    // Top border
    println!(
        "\n{}{}╔{}╗{}",
        BOLD, BLUE,
        "═".repeat(total_width - 2),
        RESET
    );

    // Title
    let title = "TASK LOGS";
    let padding_total = total_width - 2 - title.len();
    let padding_left = padding_total / 2;
    let padding_right = padding_total - padding_left;
    println!(
        "{}{}║{}{}{}║{}",
        BOLD, BLUE,
        " ".repeat(padding_left), title, " ".repeat(padding_right),
        RESET
    );

    // Header border
    println!(
        "{}{}╠{}╦{}╦{}╣{}",
        BOLD, BLUE,
        "═".repeat(id_width),
        "═".repeat(completion_width),
        "═".repeat(duration_width),
        RESET
    );

    if logs.is_empty() {
        println!(
            "{bold}{blue}║{msg:<width$}║{reset}",
            bold = BOLD,
            blue = BLUE,
            msg = " No logs found",
            width = total_width - 4,
            reset = RESET
        );
    } else {
        // Column headers
        println!(
            "{bold}{blue}║{id:<idw$}║{comp:<cw$}║{dur:<dw$}║{reset}",
            bold = BOLD,
            blue = BLUE,
            id = "TASK ID",
            idw = id_width,
            comp = "COMPLETION TIME",
            cw = completion_width,
            dur = "DURATION",
            dw = duration_width,
            reset = RESET
        );

        // Header/content separator
        println!(
            "{}{}╠{}╬{}╬{}╣{}",
            BOLD, BLUE,
            "═".repeat(id_width),
            "═".repeat(completion_width),
            "═".repeat(duration_width),
            RESET
        );

        for (id, end_time, elapsed_time) in logs {
            println!(
                "{bold}{blue}║{white}{id:<idw$}{blue}║{white}{end:<cw$}{blue}║{white}{elapsed:<dw$}{blue}║{reset}",
                bold = BOLD,
                blue = BLUE,
                white = WHITE,
                id = id,
                idw = id_width,
                end = end_time.format("%Y-%m-%d %H:%M:%S"),
                cw = completion_width,
                elapsed = format_duration_hundredths(Duration::milliseconds(*elapsed_time)),
                dw = duration_width,
                reset = RESET
            );
        }
    }

    // Bottom border
    println!(
        "{}{}╚{}╝{}",
        BOLD, BLUE,
        "═".repeat(total_width - 2),
        RESET
    );
}

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
                    .map(|elapsed| {
                        format!("{}. Elapsed time: {}{}{}", GREEN, WHITE, elapsed, GREEN)
                    })
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

        Commands::Logs { limit, id } => {
            let logs = get_task_logs(&conn, id, limit)?;

            if !cli.quiet {
                print_task_logs(&logs);
            }
        }

        Commands::Status { id } => {
            let tasks = get_all_tasks(&conn, id)?;

            if !cli.quiet {
                print_task_status(&tasks);
            }
        }
    }

    Ok(())
}

