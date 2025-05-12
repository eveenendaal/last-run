use crate::format::format_duration_hundredths;
use chrono::{DateTime, Duration, Utc};

// ANSI color constants
pub const BOLD: &str = "\x1b[1m";
pub const RESET: &str = "\x1b[0m";
pub const GREEN: &str = "\x1b[32m";
pub const RED: &str = "\x1b[31m";
pub const WHITE: &str = "\x1b[97m";
pub const BLUE: &str = "\x1b[34m";
pub const YELLOW: &str = "\x1b[33m";

/// Format and print task status
pub fn print_task_status(
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
    let elapsed_width = 10;

    // Find the maximum ID length
    for (id, _, _, _) in tasks {
        id_width = id_width.max(id.len());
    }
    // Add 4: 2 for padding, 2 for extra spaces
    id_width += 4;

    // Calculate total width
    let total_width = id_width + last_run_width + started_width + elapsed_width + 5; // 5 for borders

    // Top border
    println!(
        "\n{}{}╔{}╗{}",
        BOLD,
        BLUE,
        "═".repeat(total_width + 2),
        RESET
    );

    // Title
    let title = "TASK STATUS";
    let padding_total = total_width + 2 - title.len();
    let padding_left = padding_total / 2;
    let padding_right = padding_total - padding_left;
    println!(
        "{}{}║{}{}{}║{}",
        BOLD,
        BLUE,
        " ".repeat(padding_left),
        title,
        " ".repeat(padding_right),
        RESET
    );

    // Header border
    println!(
        "{}{}╠{}╦{}╦{}╦{}╣{}",
        BOLD,
        BLUE,
        "═".repeat(id_width),
        "═".repeat(last_run_width + 2),
        "═".repeat(started_width + 2),
        "═".repeat(elapsed_width),
        RESET
    );

    if tasks.is_empty() {
        println!(
            "{bold}{blue}║{msg:<width$}║{reset}",
            bold = BOLD,
            blue = BLUE,
            msg = " No tasks found ",
            width = total_width - 4,
            reset = RESET
        );
    } else {
        // Column headers
        println!(
            "{bold}{blue}║ {id:<idw$} ║ {last:<lrw$} ║ {start:<stw$} ║ {elapsed:<elw$} ║{reset}",
            bold = BOLD,
            blue = BLUE,
            id = "TASK ID",
            idw = id_width - 2,
            last = "LAST RUN",
            lrw = last_run_width,
            start = "STARTED",
            stw = started_width,
            elapsed = "ELAPSED",
            elw = elapsed_width - 2,
            reset = RESET
        );

        // Header/content separator
        println!(
            "{}{}╠{}╬{}╬{}╬{}╣{}",
            BOLD,
            BLUE,
            "═".repeat(id_width),
            "═".repeat(last_run_width + 2),
            "═".repeat(started_width + 2),
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
                    WHITE
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
                "{bold}{blue}║ {color}{id:<idw$}{blue} ║ {color}{last:<lrw$}{blue} ║ {color}{start:<stw$}{blue} ║ {color}{elapsed:<elw$}{blue} ║{reset}",
                bold = BOLD,
                blue = BLUE,
                color = status_color,
                id = id,
                idw = id_width - 2,
                last = last_run_str,
                lrw = last_run_width - 2,
                start = start_time_str,
                stw = started_width - 2,
                elapsed = elapsed_str,
                elw = elapsed_width - 2,
                reset = RESET
            );
        }
    }

    // Bottom border
    println!("{}{}╚{}╝{}", BOLD, BLUE, "═".repeat(total_width + 2), RESET);
}

/// Format and print task logs
pub fn print_task_logs(logs: &[(String, DateTime<Utc>, i64)]) {
    // Calculate dynamic column widths
    let mut id_width = 12; // Minimum width
    let completion_width = 21;
    let duration_width = 10;

    // Find the maximum ID length
    for (id, _, _) in logs {
        id_width = id_width.max(id.len());
    }
    // Add 4: 2 for padding, 2 for extra spaces
    id_width += 2;

    // Calculate total width
    let total_width = id_width + completion_width + duration_width + 4; // 4 for borders

    // Top border
    println!(
        "\n{}{}╔{}╗{}",
        BOLD,
        BLUE,
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
        BOLD,
        BLUE,
        " ".repeat(padding_left),
        title,
        " ".repeat(padding_right),
        RESET
    );

    // Header border
    println!(
        "{}{}╠{}╦{}╦{}╣{}",
        BOLD,
        BLUE,
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
            msg = " No logs found ",
            width = total_width - 4,
            reset = RESET
        );
    } else {
        // Column headers
        println!(
            "{bold}{blue}║ {id:<idw$} ║ {comp:<cw$} ║ {dur:<dw$} ║{reset}",
            bold = BOLD,
            blue = BLUE,
            id = "TASK ID",
            idw = id_width - 2,
            comp = "COMPLETION TIME",
            cw = completion_width - 2,
            dur = "DURATION",
            dw = duration_width - 2,
            reset = RESET
        );

        // Header/content separator
        println!(
            "{}{}╠{}╬{}╬{}╣{}",
            BOLD,
            BLUE,
            "═".repeat(id_width),
            "═".repeat(completion_width),
            "═".repeat(duration_width),
            RESET
        );

        for (id, end_time, elapsed_time) in logs {
            println!(
                "{bold}{blue}║ {white}{id:<idw$}{blue} ║ {white}{end:<cw$}{blue} ║ {white}{elapsed:<dw$}{blue} ║{reset}",
                bold = BOLD,
                blue = BLUE,
                white = WHITE,
                id = id,
                idw = id_width - 2,
                end = end_time.format("%Y-%m-%d %H:%M:%S"),
                cw = completion_width - 2,
                elapsed = format_duration_hundredths(Duration::milliseconds(*elapsed_time)),
                dw = duration_width - 2,
                reset = RESET
            );
        }
    }

    // Bottom border
    println!("{}{}╚{}╝{}", BOLD, BLUE, "═".repeat(total_width - 2), RESET);
}
