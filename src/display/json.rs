use crate::format::format_duration;
use chrono::{DateTime, Duration, Utc};
use serde_json::{json, Value};

/// Format and print task status as JSON
pub fn print_task_status_json(
    tasks: &[(String, Option<DateTime<Utc>>, Option<DateTime<Utc>>, Option<i64>)],
) {
    let now = Utc::now();

    let json_tasks: Vec<Value> = tasks.iter().map(|(id, last_run, start_time, duration)| {
        let time_since_last_run = last_run.map(|lr| {
            now.signed_duration_since(lr).num_milliseconds()
        });

        let elapsed_time = match (*start_time, *last_run) {
            (Some(st), Some(lr)) if st < lr => Some(lr.signed_duration_since(st).num_milliseconds()),
            (Some(st), None) => Some(now.signed_duration_since(st).num_milliseconds()),
            _ => None,
        };

        let status = if start_time.is_some() && last_run.is_none() {
            "running"
        } else if let Some(lr) = last_run {
            if let Some(d) = duration {
                if now.signed_duration_since(*lr) > Duration::seconds(*d) {
                    "due"
                } else {
                    "ok"
                }
            } else {
                "ok"
            }
        } else {
            "unknown"
        };

        json!({
            "id": id,
            "last_run": last_run.map(|dt| dt.to_rfc3339()),
            "time_since_last_run": time_since_last_run,
            "time_since_last_run_formatted": last_run.map(|lr| format_duration(now.signed_duration_since(lr))),
            "start_time": start_time.map(|dt| dt.to_rfc3339()),
            "elapsed_time": elapsed_time,
            "elapsed_time_formatted": elapsed_time.map(|et| format_duration(Duration::milliseconds(et))),
            "duration": duration,
            "duration_formatted": duration.map(|d| format_duration(Duration::seconds(d))),
            "status": status
        })
    }).collect();

    let output = json!({
        "tasks": json_tasks,
        "timestamp": now.to_rfc3339()
    });

    println!("{}", serde_json::to_string_pretty(&output).unwrap());
}
