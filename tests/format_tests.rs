use chrono::{Duration, TimeZone, Utc};
use lastrun::format::{format_datetime, format_duration, parse_duration};

#[test]
fn test_parse_duration() {
    // Test hours
    assert_eq!(parse_duration("5h").unwrap(), Duration::hours(5));
    assert_eq!(parse_duration("24h").unwrap(), Duration::hours(24));
    assert_eq!(parse_duration("0h").unwrap(), Duration::hours(0));
    
    // Test days
    assert_eq!(parse_duration("1d").unwrap(), Duration::days(1));
    assert_eq!(parse_duration("7d").unwrap(), Duration::days(7));
    assert_eq!(parse_duration("30d").unwrap(), Duration::days(30));
    
    // Test invalid inputs
    assert!(parse_duration("5m").is_err()); // Minutes not supported
    assert!(parse_duration("5").is_err());  // No unit
    assert!(parse_duration("ab").is_err()); // Not a number
    assert!(parse_duration("").is_err());   // Empty string
}

#[test]
fn test_format_duration() {
    // Test various duration formats
    assert_eq!(format_duration(Duration::minutes(5)), "5m0s");
    assert_eq!(format_duration(Duration::minutes(65)), "1h5m");
    assert_eq!(format_duration(Duration::hours(5)), "5h0m");
    assert_eq!(format_duration(Duration::hours(25)), "1d1h");
    assert_eq!(format_duration(Duration::days(1)), "1d0h");
    assert_eq!(format_duration(Duration::days(2) + Duration::hours(12) + Duration::minutes(30)), "2d12h");

    // New: test seconds to hundredths
    assert_eq!(format_duration(Duration::milliseconds(1500)), "1.50s");
    assert_eq!(format_duration(Duration::milliseconds(500)), "0.50s");
    assert_eq!(format_duration(Duration::milliseconds(10)), "0.01s");
    assert_eq!(format_duration(Duration::milliseconds(1)), "0.00s");
    assert_eq!(format_duration(Duration::seconds(5) + Duration::milliseconds(250)), "5.25s");
    assert_eq!(format_duration(Duration::seconds(60) + Duration::milliseconds(750)), "1m0s");
}

#[test]
fn test_format_datetime() {
    // Create a fixed UTC datetime for consistent testing
    let test_dt = Utc.with_ymd_and_hms(2023, 5, 15, 12, 0, 0).unwrap();
    
    // The result will depend on the local timezone of the running machine
    // We'll just check that it returns a string of the expected format
    let formatted = format_datetime(&test_dt);
    
    // The string should be in format: YYYY-MM-DD HH:MM:SS
    assert!(formatted.len() >= 19); // At least this many characters
    
    // Check it has the expected date part (should be consistent regardless of timezone)
    assert!(formatted.starts_with("2023-05-15"));
    
    // Check that it contains the time separators
    assert!(formatted.contains(":"));
}
