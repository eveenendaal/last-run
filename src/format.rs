use chrono::{DateTime, Duration, Local, Utc};

pub fn parse_duration(duration_str: &str) -> Result<Duration, String> {
    if let Some(hours_str) = duration_str.strip_suffix('h') {
        if let Ok(hours) = hours_str.parse::<i64>() {
            return Ok(Duration::hours(hours));
        }
    } else if let Some(days_str) = duration_str.strip_suffix('d') {
        if let Ok(days) = days_str.parse::<i64>() {
            return Ok(Duration::days(days));
        }
    } else if let Some(weeks_str) = duration_str.strip_suffix('w') {
        if let Ok(weeks) = weeks_str.parse::<i64>() {
            return Ok(Duration::weeks(weeks));
        }
    } else if let Some(months_str) = duration_str.strip_suffix('m') {
        if let Ok(months) = months_str.parse::<i64>() {
            return Ok(Duration::days(months * 30));
        }
    }

    Err(format!(
        "Invalid duration format: '{}'. Use a number followed by h (hours), d (days), w (weeks), or m (months). Examples: 24h, 7d, 2w, 3m",
        duration_str
    ))
}

pub fn format_duration(duration: Duration) -> String {
    let total_seconds = duration.num_seconds().abs();
    let sign = if duration.num_seconds() < 0 { "-" } else { "" };

    if total_seconds < 60 {
        // Show seconds to hundredths
        let total_milliseconds = duration.num_milliseconds().abs();
        let seconds = total_milliseconds / 1000;
        let hundredths = (total_milliseconds % 1000) / 10;
        format!("{}{:.0}.{:02}s", sign, seconds, hundredths)
    } else if total_seconds < 3600 {
        // Show XmYs
        let minutes = total_seconds / 60;
        let seconds = total_seconds % 60;
        format!("{}{}m{}s", sign, minutes, seconds)
    } else {
        let total_minutes = total_seconds / 60;
        let days = total_minutes / (24 * 60);
        let hours = (total_minutes % (24 * 60)) / 60;
        let minutes = total_minutes % 60;

        if days > 0 {
            // If more than a day, don't show minutes
            format!("{}{}d{}h", sign, days, hours)
        } else {
            format!("{}{}h{}m", sign, hours, minutes)
        }
    }
}

/// Parse an optional RFC3339 string into an optional `DateTime<Utc>`, silently discarding errors.
pub fn parse_rfc3339_opt(s: Option<String>) -> Option<DateTime<Utc>> {
    s.as_deref()
        .and_then(|s| DateTime::parse_from_rfc3339(s).ok())
        .map(|dt| dt.with_timezone(&Utc))
}

/// Format a DateTime<Utc> to "%Y-%m-%d %H:%M:%S" in the local timezone
pub fn format_datetime(datetime: &DateTime<Utc>) -> String {
    let local_datetime = datetime.with_timezone(&Local);
    local_datetime.format("%Y-%m-%d %H:%M:%S").to_string()
}
