use crate::format::format_duration;
use chrono::{DateTime, Duration, Utc};
use clap::{Parser, Subcommand};

#[derive(Parser)]
#[command(name = "lastrun")]
#[command(about = "A utility to track when tasks were last run")]
pub struct Cli {
    /// Suppress output messages
    #[arg(short, long)]
    pub quiet: bool,

    #[command(subcommand)]
    pub command: Commands,
}

#[derive(Subcommand)]
pub enum Commands {
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

    /// Reset the tasks database
    Clean {

    }
}

pub fn should_run_task(last_run: DateTime<Utc>, duration: Duration) -> (bool, String) {
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
