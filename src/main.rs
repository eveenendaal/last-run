mod cli;
mod db;
mod display;
mod error;
mod format;
mod model;

use chrono::Utc;
use clap::Parser;
use cli::{should_run_task, Cli, Commands};
use db::update_task_duration;
use display::{print_task_logs, print_task_status, BOLD, GREEN, RED, RESET, WHITE};
use error::{AppError, AppResult};
use format::{format_datetime, format_duration, parse_duration};
use model::Task;
use std::io::{self, Write};
use std::process;
use std::{thread, time::Duration};

fn require_id(id: &str) -> AppResult<()> {
    if id.is_empty() {
        Err(AppError::MissingTaskId)
    } else {
        Ok(())
    }
}

fn main() -> AppResult<()> {
    let cli = Cli::parse();

    let conn = db::get_file_based_connection()?;
    db::init_db(&conn)?;

    match cli.command {
        Commands::Update { id } | Commands::Done { id } => {
            require_id(&id)?;

            let mut elapsed_time = None;
            let mut task = Task::ensure(&conn, &id, cli.quiet)?;
            task.last_run = Some(Utc::now());
            task.update(&conn)?;
            if let Some(start_time) = task.start_time {
                elapsed_time = Some(format_duration(
                    Utc::now().signed_duration_since(start_time),
                ));
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
            require_id(&id)?;

            let mut task = Task::ensure(&conn, &id, cli.quiet)?;
            task.start_time = Some(Utc::now());
            task.last_run = None;
            task.update(&conn)?;

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
            require_id(&id)?;

            let duration = parse_duration(&duration).map_err(AppError::DurationParse)?;
            let task = match Task::select(&conn, &id, cli.quiet)? {
                Some(task) => task,
                None => {
                    if !cli.quiet {
                        println!(
                            "{}{}Task {}{}{} does not exist yet. It is considered due.{}",
                            BOLD, RED, WHITE, id, RED, RESET
                        );
                    }
                    process::exit(1);
                }
            };

            let should_exit_due = match task.last_run {
                Some(last_run) => {
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
                    update_task_duration(&conn, &task.id, duration.num_seconds())?;
                    should_run
                }
                None => {
                    if !cli.quiet {
                        println!(
                            "{}{}Task {}{}{} has no recorded last run. It is considered due.{}",
                            BOLD, RED, WHITE, task.id, RED, RESET
                        );
                    }
                    true
                }
            };

            if should_exit_due {
                process::exit(1);
            }
        }

        Commands::Logs { limit, id } => {
            let logs = db::get_task_logs(&conn, id, limit)?;
            if !cli.quiet {
                print_task_logs(&logs);
            }
        }

        Commands::Status { id, watch, sort } => {
            let draw_interval_ms = 100;
            let clear_interval_secs = 5;
            let interval = Duration::from_millis(draw_interval_ms);
            let clears_every = (clear_interval_secs * 1000) / draw_interval_ms;
            let mut first = true;
            let mut ticks = 0;
            loop {
                if watch {
                    if first || ticks % clears_every == 0 {
                        print!("\x1B[2J\x1B[H");
                        first = false;
                    } else {
                        print!("\x1B[H");
                    }
                    io::stdout().flush().unwrap();
                }

                let tasks = db::get_all_tasks(&conn, id.as_ref().cloned())?;
                if !cli.quiet {
                    print_task_status(&tasks, &sort);
                }
                if !watch {
                    break;
                }
                thread::sleep(interval);
                ticks += 1;
            }
        }

        Commands::Reset {} => {
            db::clean_db(&conn)?;
            if !cli.quiet {
                println!("{}{}Tasks table has been rebuilt.{}", BOLD, GREEN, RESET);
            }
        }

        Commands::Delete { id } => {
            require_id(&id)?;

            let logs_deleted = db::delete_task_logs(&conn, &id)?;
            let task_deleted = db::delete_task(&conn, &id)?;

            if !cli.quiet {
                if task_deleted > 0 {
                    println!(
                        "{}{}Task {}{}{} deleted. {} log entries removed.{}",
                        BOLD, GREEN, WHITE, id, GREEN, logs_deleted, RESET
                    );
                } else {
                    println!(
                        "{}{}No task found with ID: {}{}{}. {} log entries removed.{}",
                        BOLD, RED, WHITE, id, RED, logs_deleted, RESET
                    );
                }
            }
        }

        Commands::Clear { id } => {
            require_id(&id)?;
            let mut task = match Task::select(&conn, &id, cli.quiet)? {
                Some(task) => task,
                None => {
                    if !cli.quiet {
                        println!(
                            "{}{}Task {}{}{} does not exist.{}",
                            BOLD, RED, WHITE, id, RED, RESET
                        );
                    }
                    return Ok(());
                }
            };
            task.last_run = None;
            task.start_time = None;
            task.update(&conn)?;
            if !cli.quiet {
                println!(
                    "{}{}Task {}{}{} cleared (start and done values reset).{}",
                    BOLD, GREEN, WHITE, id, GREEN, RESET
                );
            }
        }
        // Add a new subcommand for completions
        Commands::Completions { shell } => {
            cli::generate_completions(shell);
        }
    }

    Ok(())
}
