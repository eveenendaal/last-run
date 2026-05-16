use crate::format::{format_datetime, format_duration};
use chrono::{DateTime, Duration, Utc};
use prettytable::{format, Cell, Row, Table};

const HEADER_COLOR: &str = "FG";
const TEXT_COLOR: &str = "FW";

/// Format and print task logs
pub fn print_task_logs(logs: &[(String, DateTime<Utc>, i64)]) {
    let mut table = Table::new();

    table.set_format(*format::consts::FORMAT_NO_LINESEP_WITH_TITLE);

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
