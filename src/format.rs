use chrono::{Duration, DateTime, Utc, Local};

pub fn parse_duration(duration_str: &str) -> Result<Duration, String> {
    if let Some(hours_str) = duration_str.strip_suffix('h') {
        if let Ok(hours) = hours_str.parse::<i64>() {
            return Ok(Duration::hours(hours));
        }
    } else if let Some(days_str) = duration_str.strip_suffix('d') {
        if let Ok(days) = days_str.parse::<i64>() {
            return Ok(Duration::days(days));
        }
    }

    Err(format!("Invalid duration format: {}", duration_str))
}

pub fn format_duration(duration: Duration) -> String {
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

pub fn format_duration_hundredths(duration: Duration) -> String {
    let total_milliseconds = duration.num_milliseconds();
    let seconds = total_milliseconds / 1000;
    let hundredths = (total_milliseconds % 1000) / 10;

    format!("{}.{}s", seconds, hundredths)
}

/// Format a DateTime<Utc> to "%Y-%m-%d %H:%M:%S" in the local timezone
pub fn format_datetime(datetime: &DateTime<Utc>) -> String {
    let local_datetime = datetime.with_timezone(&Local);
    local_datetime.format("%Y-%m-%d %H:%M:%S").to_string()
}

