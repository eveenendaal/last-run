use crate::format::{format_datetime, format_duration};
use chrono::{DateTime, Duration, Utc};
use prettytable::{format, Cell, Row, Table};

// ANSI color constants
pub const BOLD: &str = "\x1b[1m";
pub const RESET: &str = "\x1b[0m";
pub const GREEN: &str = "\x1b[32m";
pub const RED: &str = "\x1b[31m";
pub const WHITE: &str = "\x1b[97m";
pub const HEADER_COLOR: &str = "FG";
pub const TEXT_COLOR: &str = "FW";

/// Format and print task status
pub fn print_task_status(
    tasks: &[(String, Option<DateTime<Utc>>, Option<DateTime<Utc>>, Option<i64>)],
    sort_by: &str,
) {
    let mut tasks = tasks.to_vec();
    let now = Utc::now();

    // Sort tasks based on the sort_by parameter
    match sort_by {
        "id" => tasks.sort_by(|a, b| a.0.cmp(&b.0)),
        "last_run" => tasks.sort_by(|a, b| a.1.cmp(&b.1)),
        "time_since_last_run" => tasks.sort_by(|a, b| {
            let a_val = a.1.map(|lr| now.signed_duration_since(lr)).unwrap_or(chrono::Duration::MAX);
            let b_val = b.1.map(|lr| now.signed_duration_since(lr)).unwrap_or(chrono::Duration::MAX);
            a_val.cmp(&b_val)
        }),
        "started" => tasks.sort_by(|a, b| a.2.cmp(&b.2)),
        "elapsed" => tasks.sort_by(|a, b| {
            let a_val = match (a.2, a.1) {
                (Some(st), Some(lr)) if st < lr => lr.signed_duration_since(st),
                (Some(st), None) => now.signed_duration_since(st),
                _ => chrono::Duration::zero(),
            };
            let b_val = match (b.2, b.1) {
                (Some(st), Some(lr)) if st < lr => lr.signed_duration_since(st),
                (Some(st), None) => now.signed_duration_since(st),
                _ => chrono::Duration::zero(),
            };
            a_val.cmp(&b_val)
        }),
        "duration" => tasks.sort_by(|a, b| a.3.cmp(&b.3)),
        _ => {} // Default: no sorting
    }

    let mut table = Table::new();
    let now = Utc::now();

    // Set table formatting
    table.set_format(*format::consts::FORMAT_NO_LINESEP_WITH_TITLE);

    // Set title
    table.set_titles(Row::new(vec![
        Cell::new("TASK ID").style_spec(HEADER_COLOR),
        Cell::new("LAST RUN").style_spec(HEADER_COLOR),
        Cell::new("TIME SINCE LAST RUN").style_spec(HEADER_COLOR),
        Cell::new("STARTED").style_spec(HEADER_COLOR),
        Cell::new("ELAPSED").style_spec(HEADER_COLOR),
        Cell::new("DURATION").style_spec(HEADER_COLOR),
    ]));

    if tasks.is_empty() {
        let empty_row = Row::new(vec![Cell::new("No tasks found")
            .with_hspan(6)
            .style_spec("c")]);
        table.add_row(empty_row);
    } else {
        for (id, last_run, start_time, duration) in &tasks {
            let status_color = if start_time.is_some() && last_run.is_none() {
                "Fy" // Yellow
            } else if let Some(lr) = last_run {
                let time_since_last = now.signed_duration_since(*lr);
                
                // Only show in red if duration is set and time since last run exceeds that duration
                if let Some(d) = duration {
                    if time_since_last > Duration::seconds(*d) {
                        "Fr" // Red
                    } else {
                        TEXT_COLOR // White
                    }
                } else {
                    TEXT_COLOR // White
                }
            } else {
                "Fb" // Blue
            };

            let last_run_str = if let Some(lr) = last_run {
                format_datetime(lr)
            } else {
                "never".to_string()
            };

            // Calculate time since last run
            let time_since_last_run = if let Some(lr) = last_run {
                format_duration(now.signed_duration_since(*lr))
            } else {
                "-".to_string()
            };

            let start_time_str = if let Some(st) = start_time {
                format_datetime(st)
            } else {
                "-".to_string()
            };

            // Calculate elapsed time since start
            let elapsed_str = if let Some(st) = start_time {
                if let Some(lr) = last_run {
                    if *st < *lr {
                        // Task has completed, show elapsed time from start to last_run
                        let elapsed = lr.signed_duration_since(*st);
                        format_duration(elapsed)
                    } else {
                        // Invalid state (start time after last run)
                        "-".to_string()
                    }
                } else {
                    // Task is still running, show elapsed time from start until now
                    let elapsed = now.signed_duration_since(*st);
                    format_duration(elapsed)
                }
            } else {
                "-".to_string()
            };

            let duration_str = if let Some(d) = duration {
                format_duration(Duration::seconds(*d))
            } else {
                "-".to_string()
            };

            table.add_row(Row::new(vec![
                Cell::new(id).style_spec(status_color),
                Cell::new(&last_run_str).style_spec(status_color),
                Cell::new(&time_since_last_run).style_spec(status_color),
                Cell::new(&start_time_str).style_spec(status_color),
                Cell::new(&elapsed_str).style_spec(status_color),
                Cell::new(&duration_str).style_spec(status_color),
            ]));
        }
    }

    table.printstd();
}

/// Format and print task logs
pub fn print_task_logs(logs: &[(String, DateTime<Utc>, i64)]) {
    let mut table = Table::new();

    // Set table formatting
    table.set_format(*format::consts::FORMAT_NO_LINESEP_WITH_TITLE);

    // Set title
    table.set_titles(Row::new(vec![
        Cell::new("TASK ID").style_spec(HEADER_COLOR),
        Cell::new("COMPLETION TIME").style_spec(HEADER_COLOR),
        Cell::new("DURATION").style_spec(HEADER_COLOR),
    ]));

    if logs.is_empty() {
        let empty_row = Row::new(vec![Cell::new("No logs found")
            .with_hspan(3)
            .style_spec("c")]);
        table.add_row(empty_row);
    } else {
        for (id, end_time, elapsed_time) in logs {
            let duration = Duration::milliseconds(*elapsed_time);
            let duration_str = format_duration(duration);
            table.add_row(Row::new(vec![
                Cell::new(id).style_spec(TEXT_COLOR),
                Cell::new(&format_datetime(end_time)).style_spec(TEXT_COLOR),
                Cell::new(&duration_str).style_spec(TEXT_COLOR),
            ]));
        }
    }

    table.printstd();
}

