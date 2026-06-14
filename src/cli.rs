use crate::format::format_duration;
use chrono::{DateTime, Duration, Utc};
use clap::{CommandFactory, Parser, Subcommand, ValueEnum};

#[derive(Parser)]
#[command(author, name = "lastrun", version = env!("APP_VERSION"), about = "A utility to track when tasks were last run", long_about = None)]
pub struct Cli {
    /// Suppress output messages
    #[arg(short, long)]
    pub quiet: bool,

    #[command(subcommand)]
    pub command: Commands,
}

#[derive(clap::ValueEnum, Clone, Debug)]
pub enum Shell {
    Zsh,
}

#[derive(ValueEnum, Clone, Debug)]
pub enum SortColumn {
    Task,
    Status,
    Duration,
    Elapsed,
    LastRun,
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

        /// Column to sort by (task, status, duration, elapsed, last-run)
        #[arg(short, long, value_enum, default_value_t = SortColumn::LastRun)]
        sort: SortColumn,

        /// Output status in JSON format
        #[arg(long)]
        json: bool,
    },

    /// Reset the tasks database
    Reset {},

    /// Delete a task and its log records by ID
    Delete {
        /// Task ID to delete
        #[arg(short, long)]
        id: String,
    },

    /// Clear a task's start and done values
    Clear {
        /// Task ID to clear
        #[arg(short, long)]
        id: String,
    },

    /// Delete log entries older than a specified period
    Archive {
        /// How far back to keep logs (e.g. 30d, 2w, 3m, 24h). Entries older than this are deleted.
        #[arg(short, long)]
        older_than: String,

        /// Limit archiving to a specific task ID (default: all tasks)
        #[arg(short, long)]
        id: Option<String>,

        /// Skip the confirmation prompt and delete immediately
        #[arg(short, long)]
        yes: bool,
    },

    /// Generate shell completion for your shell
    Completion {
        /// The shell to generate completions for (zsh)
        #[arg(value_enum)]
        shell: Shell,
    },
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

// Add a function to generate shell completions
pub fn generate_completions(shell: Shell) {
    use clap_complete;
    use std::io;
    let mut cmd = Cli::command();
    let bin_name = cmd.get_name().to_string();
    clap_complete::generate(
        match shell {
            Shell::Zsh => clap_complete::Shell::Zsh,
        },
        &mut cmd,
        bin_name,
        &mut io::stdout(),
    );
}
