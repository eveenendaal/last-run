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
use display::{print_task_logs, print_task_status_json, run_tui, SortCol, BOLD, GREEN, RED, RESET, WHITE};
use error::{AppError, AppResult};
use format::{format_datetime, format_duration, parse_duration};
use model::Task;
use std::io::{self, Write};
use std::process;

fn require_id(id: &str) -> AppResult<()> {
    if id.is_empty() {
        Err(AppError::MissingTaskId)
    } else {
        Ok(())
    }
}

fn main() -> AppResult<()> {
    let cli = Cli::parse();

    let conn = db::get_file_based_connection(cli.db_path)?;
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
            let task = match Task::select(&conn, &id)? {
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

        Commands::Status { id, sort, json } => {
            if json {
                let tasks = db::get_all_tasks(&conn, id)?;
                if !cli.quiet {
                    print_task_status_json(&tasks);
                }
            } else {
                let sort_col = match sort {
                    cli::SortColumn::Task => SortCol::Task,
                    cli::SortColumn::Status => SortCol::Status,
                    cli::SortColumn::Duration => SortCol::Duration,
                    cli::SortColumn::Elapsed => SortCol::Elapsed,
                    cli::SortColumn::LastRun => SortCol::LastRun,
                };
                run_tui(&conn, id, sort_col)?;
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
            let mut task = match Task::select(&conn, &id)? {
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
        Commands::Archive { older_than, id, yes } => {
            let duration = parse_duration(&older_than).map_err(AppError::DurationParse)?;
            let cutoff = Utc::now() - duration;
            let task_id_ref = id.as_deref();

            let count = db::count_old_logs(&conn, &cutoff, task_id_ref)?;

            if count == 0 {
                if !cli.quiet {
                    println!(
                        "{}{}No log entries found older than {}.{}",
                        BOLD, GREEN, older_than, RESET
                    );
                }
                return Ok(());
            }

            if !cli.quiet {
                let scope = match &id {
                    Some(task_id) => format!(" for task {WHITE}{task_id}{GREEN}"),
                    None => String::from(" across all tasks"),
                };
                println!(
                    "{}{}Archive logs{scope}{}",
                    BOLD, GREEN, RESET
                );
                println!(
                    "  Keeping entries from: {}{}{}",
                    WHITE,
                    cutoff.format("%Y-%m-%d"),
                    RESET
                );
                println!(
                    "  Entries to delete:    {}{}{}{}",
                    WHITE, BOLD, count, RESET
                );
                println!();
            }

            let confirmed = yes || {
                print!("Permanently delete {count} log {entries}? [y/N]: ",
                    entries = if count == 1 { "entry" } else { "entries" }
                );
                io::stdout().flush()?;
                let mut input = String::new();
                io::stdin().read_line(&mut input)?;
                matches!(input.trim().to_lowercase().as_str(), "y" | "yes")
            };

            if confirmed {
                let deleted = db::delete_old_logs(&conn, &cutoff, task_id_ref)?;
                if !cli.quiet {
                    println!(
                        "{}{}Deleted {} log {}.{}",
                        BOLD,
                        GREEN,
                        deleted,
                        if deleted == 1 { "entry" } else { "entries" },
                        RESET
                    );
                }
            } else if !cli.quiet {
                println!("{}Cancelled.{}", RED, RESET);
            }
        }

        // Add a new subcommand for completions
        Commands::Completion { shell } => {
            cli::generate_completions(shell);
        }
    }

    Ok(())
}
