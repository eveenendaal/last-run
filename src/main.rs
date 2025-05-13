mod cli;
mod db;
mod display;
mod error;
mod format;
mod model;

use chrono::Utc;
use clap::Parser;
use cli::{should_run_task, Cli, Commands};
use display::{print_task_logs, print_task_status, BOLD, GREEN, RED, RESET, WHITE};
use error::{AppError, AppResult};
use format::{format_duration_hundredths, parse_duration, format_datetime};
use model::Task;
use std::process;

fn main() -> AppResult<()> {
    let cli = Cli::parse();

    let conn = db::get_file_based_connection()?;
    db::init_db(&conn)?; // Pass the connection to initialize the schema

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
                    format_datetime(&task.last_run.unwrap()),
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
                    format_datetime(&task.start_time.unwrap()),
                    RESET
                );
            }
        }

        Commands::Check { id, duration } => {
            if id.is_empty() {
                return Err(AppError::MissingTaskId);
            }

            let duration = parse_duration(&duration).map_err(AppError::DurationParse)?;

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
            let logs = db::get_task_logs(&conn, id, limit)?;

            if !cli.quiet {
                print_task_logs(&logs);
            }
        }

        Commands::Status { id } => {
            let tasks = db::get_all_tasks(&conn, id)?;

            if !cli.quiet {
                print_task_status(&tasks);
            }
        }
    }

    Ok(())
}
