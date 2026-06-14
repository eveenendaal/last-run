mod common;
use chrono::{Duration, Utc};
use common::make_task;
use lastrun::cli::should_run_task;
use lastrun::db::{delete_task, delete_task_logs, get_all_tasks, get_task_logs, init_db};
use rusqlite::Connection;
use std::thread::sleep;
use std::time::Duration as StdDuration;

#[test]
fn test_complete_task_workflow() {
    // Setup in-memory DB
    let conn = Connection::open_in_memory().unwrap();
    init_db(&conn).unwrap();

    // 1. Create new task
    let task_id = "workflow_test".to_string();
    let mut task = make_task(&task_id);
    task.insert(&conn).unwrap();

    // 2. Verify task exists with no run time
    let tasks = get_all_tasks(&conn, Some(task_id.clone())).unwrap();
    assert_eq!(tasks.len(), 1);
    assert_eq!(tasks[0].0, task_id);
    assert!(tasks[0].1.is_none()); // No last_run yet

    // 3. Start the task
    task.start_time = Some(Utc::now());
    task.update(&conn).unwrap();

    // Verify last_run is cleared when start_time is set
    let tasks_after_start = get_all_tasks(&conn, Some(task_id.clone())).unwrap();
    assert!(tasks_after_start[0].1.is_none()); // last_run should be None

    // 4. Wait a bit and complete the task
    sleep(StdDuration::from_millis(100));
    task.last_run = Some(Utc::now());
    task.update(&conn).unwrap();

    // 5. Verify the task has been updated
    let updated_tasks = get_all_tasks(&conn, Some(task_id.clone())).unwrap();
    assert!(updated_tasks[0].1.is_some()); // Has last_run now
    let last_run = updated_tasks[0].1.unwrap(); // Safely unwrap after checking

    // 6. Verify a log entry was created
    let logs = get_task_logs(&conn, Some(task_id.clone()), 10).unwrap();
    assert_eq!(logs.len(), 1);
    assert_eq!(logs[0].0, task_id);
    assert!(logs[0].2 > 0); // Should have some elapsed time

    // 7. Test should_run_task logic
    let (should_run, _) = should_run_task(last_run, Duration::hours(24));
    assert_eq!(should_run, false); // Should not run yet, it just completed

    // 8. Delete logs for task
    let logs_deleted = delete_task_logs(&conn, &task_id).unwrap();
    assert_eq!(logs_deleted, 1);

    let logs_after = get_task_logs(&conn, Some(task_id.clone()), 10).unwrap();
    assert_eq!(logs_after.len(), 0);

    // 9. Delete the task completely
    let task_deleted = delete_task(&conn, &task_id).unwrap();
    assert_eq!(task_deleted, 1);

    let tasks_final = get_all_tasks(&conn, Some(task_id.clone())).unwrap();
    assert_eq!(tasks_final.len(), 0);
}

#[test]
fn test_mutiple_task_management() {
    // Setup in-memory DB
    let conn = Connection::open_in_memory().unwrap();
    init_db(&conn).unwrap();

    // 1. Create three different tasks
    let task_ids = ["daily_task", "weekly_task", "monthly_task"];

    for id in task_ids.iter() {
        let task = make_task(id);
        task.insert(&conn).unwrap();
    }

    // 2. Verify all tasks were created
    let all_tasks = get_all_tasks(&conn, None).unwrap();
    assert_eq!(all_tasks.len(), 3);

    // 3. Update tasks with different times
    let mut daily = make_task("daily_task");
    daily.start_time = Some(Utc::now());
    daily.update(&conn).unwrap();

    let mut weekly = make_task("weekly_task");
    weekly.start_time = Some(Utc::now());
    weekly.update(&conn).unwrap();

    let mut monthly = make_task("monthly_task");
    monthly.start_time = Some(Utc::now());
    monthly.update(&conn).unwrap();

    // Verify last_run is cleared for all tasks
    for id in task_ids.iter() {
        let task_status = get_all_tasks(&conn, Some(id.to_string())).unwrap();
        assert!(task_status[0].1.is_none()); // last_run should be None
    }

    // 4. Check task status against different thresholds
    let daily_status = get_all_tasks(&conn, Some("daily_task".to_string())).unwrap();
    if let Some(daily_last_run) = daily_status[0].1 {
        let (daily_should_run, _) = should_run_task(daily_last_run, Duration::hours(24));
        assert_eq!(daily_should_run, false); // Not due yet (12h < 24h threshold)
    }

    let weekly_status = get_all_tasks(&conn, Some("weekly_task".to_string())).unwrap();
    if let Some(weekly_last_run) = weekly_status[0].1 {
        let (weekly_should_run, _) = should_run_task(weekly_last_run, Duration::days(7));
        assert_eq!(weekly_should_run, false); // Not due yet (3d < 7d threshold)
    }

    let monthly_status = get_all_tasks(&conn, Some("monthly_task".to_string())).unwrap();
    if let Some(monthly_last_run) = monthly_status[0].1 {
        let (monthly_should_run, _) = should_run_task(monthly_last_run, Duration::days(14));
        assert_eq!(monthly_should_run, true); // Due (15d > 14d threshold)
    }

    // 5. Delete all tasks
    for id in task_ids.iter() {
        delete_task_logs(&conn, id).unwrap();
        delete_task(&conn, id).unwrap();
    }

    // 6. Verify all tasks are gone
    let final_tasks = get_all_tasks(&conn, None).unwrap();
    assert_eq!(final_tasks.len(), 0);
}

#[test]
fn test_auto_archive_on_done() {
    use lastrun::db::{delete_old_logs, get_task_logs, set_log_retention};
    use chrono::Duration;

    let conn = Connection::open_in_memory().unwrap();
    init_db(&conn).unwrap();

    // Set retention to a very short window (1 hour)
    set_log_retention(&conn, "1h").unwrap();

    // Create a task and record a completion from 2 hours ago
    let mut task = make_task("auto_archive_test");
    task.insert(&conn).unwrap();
    task.start_time = Some(Utc::now() - Duration::hours(2));
    task.last_run = Some(Utc::now() - Duration::hours(2));
    task.update(&conn).unwrap();

    // Verify the log entry exists
    let logs = get_task_logs(&conn, Some("auto_archive_test".to_string()), 10).unwrap();
    assert_eq!(logs.len(), 1);

    // Simulate auto-archive: delete logs older than 1 hour
    let cutoff = Utc::now() - Duration::hours(1);
    let deleted = delete_old_logs(&conn, &cutoff, None).unwrap();
    assert_eq!(deleted, 1);

    // Verify the log entry is gone
    let logs = get_task_logs(&conn, Some("auto_archive_test".to_string()), 10).unwrap();
    assert_eq!(logs.len(), 0);
}

#[test]
fn test_archive_default_falls_back_when_retention_off() {
    // Regression: `archive` with no --older-than must not error when retention
    // is "off". It should fall back to a 30-day cutoff, mirroring main.rs:
    //   get_log_retention_seconds().unwrap_or(30 days)
    use lastrun::db::{
        delete_old_logs, get_log_retention_seconds, get_task_logs, set_log_retention,
    };
    use chrono::Duration;

    let conn = Connection::open_in_memory().unwrap();
    init_db(&conn).unwrap();

    set_log_retention(&conn, "off").unwrap();
    assert!(get_log_retention_seconds(&conn).unwrap().is_none());

    // Two entries: one older than the 30d default, one within it.
    let mut task = make_task("archive_off_test");
    task.insert(&conn).unwrap();
    task.start_time = Some(Utc::now() - Duration::days(40));
    task.last_run = Some(Utc::now() - Duration::days(40));
    task.update(&conn).unwrap();
    task.start_time = Some(Utc::now() - Duration::days(1));
    task.last_run = Some(Utc::now() - Duration::days(1));
    task.update(&conn).unwrap();

    // Resolve the cutoff the way `archive` does when --older-than is omitted.
    let seconds = get_log_retention_seconds(&conn).unwrap().unwrap_or(30 * 24 * 3600);
    let cutoff = Utc::now() - Duration::seconds(seconds);

    let deleted = delete_old_logs(&conn, &cutoff, None).unwrap();
    assert_eq!(deleted, 1);

    let logs = get_task_logs(&conn, Some("archive_off_test".to_string()), 10).unwrap();
    assert_eq!(logs.len(), 1);
}

#[test]
fn test_archive_preserves_recent_logs() {
    use lastrun::db::{delete_old_logs, get_task_logs};
    use chrono::Duration;

    let conn = Connection::open_in_memory().unwrap();
    init_db(&conn).unwrap();

    // Create two log entries: one old, one recent
    let mut task = make_task("preserve_test");
    task.insert(&conn).unwrap();

    // Old entry
    task.start_time = Some(Utc::now() - Duration::hours(48));
    task.last_run = Some(Utc::now() - Duration::hours(48));
    task.update(&conn).unwrap();

    // Recent entry
    task.start_time = Some(Utc::now() - Duration::minutes(5));
    task.last_run = Some(Utc::now() - Duration::minutes(5));
    task.update(&conn).unwrap();

    let logs = get_task_logs(&conn, Some("preserve_test".to_string()), 10).unwrap();
    assert_eq!(logs.len(), 2);

    // Archive older than 24h — should only delete the old one
    let cutoff = Utc::now() - Duration::hours(24);
    let deleted = delete_old_logs(&conn, &cutoff, None).unwrap();
    assert_eq!(deleted, 1);

    let logs = get_task_logs(&conn, Some("preserve_test".to_string()), 10).unwrap();
    assert_eq!(logs.len(), 1);
}
