use chrono::{Duration, Utc};
use lastrun::cli::should_run_task;

#[test]
fn test_should_run_task() {
    // Create a fixed reference time in the past
    let now = Utc::now();
    
    // Test case: Task ran recently (10 hours ago), threshold is 24 hours
    let recent_run = now - Duration::hours(10);
    let threshold = Duration::hours(24);
    let (should_run, message) = should_run_task(recent_run, threshold);
    
    assert_eq!(should_run, false);
    assert!(message.contains("not due yet"));
    assert!(message.contains("10h"));
    
    // Test case: Task ran long ago (30 hours ago), threshold is 24 hours
    let old_run = now - Duration::hours(30);
    let (should_run, message) = should_run_task(old_run, threshold);
    
    assert_eq!(should_run, true);
    assert!(message.contains("Task is due"));
    assert!(message.contains("1d6h"));
    
    // Test case: Task ran exactly at threshold
    let threshold_run = now - Duration::hours(24);
    let (should_run, message) = should_run_task(threshold_run, threshold);
    
    assert_eq!(should_run, true);
    assert!(message.contains("Task is due"));
    
    // Test case: Task ran 6 days ago, threshold is 7 days
    let week_almost_run = now - Duration::days(6);
    let week_threshold = Duration::days(7);
    let (should_run, message) = should_run_task(week_almost_run, week_threshold);
    
    assert_eq!(should_run, false);
    assert!(message.contains("not due yet"));
    assert!(message.contains("6d"));
    
    // Test case: Task ran 8 days ago, threshold is 7 days
    let week_over_run = now - Duration::days(8);
    let (should_run, message) = should_run_task(week_over_run, week_threshold);
    
    assert_eq!(should_run, true);
    assert!(message.contains("Task is due"));
    assert!(message.contains("8d"));
}
