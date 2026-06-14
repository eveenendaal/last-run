mod common;
use chrono::Utc;
use common::make_task;
use lastrun::db::{clean_db, get_all_tasks, get_task_logs, init_db};
use lastrun::model::Task;
use rusqlite::Connection;

#[test]
fn test_database_initialization() {
    let conn = Connection::open_in_memory().unwrap();
    init_db(&conn).unwrap(); // Pass the in-memory connection

    let tables: Vec<String> = conn
        .prepare("SELECT name FROM sqlite_master WHERE type='table'")
        .unwrap()
        .query_map([], |row| row.get(0))
        .unwrap()
        .map(|table| table.unwrap())
        .collect();

    assert!(tables.contains(&"tasks".to_string()));
    assert!(tables.contains(&"task_log".to_string()));
}

#[test]
fn test_task_crud_operations() {
    let conn = Connection::open_in_memory().unwrap();
    init_db(&conn).unwrap(); // Pass the in-memory connection

    let mut task = make_task("test_task");
    task.insert(&conn).unwrap();

    let fetched_task = Task::select(&conn, "test_task").unwrap().unwrap();
    assert_eq!(fetched_task.id, "test_task");

    task.last_run = Some(Utc::now());
    task.update(&conn).unwrap();

    let updated_task = Task::select(&conn, "test_task").unwrap().unwrap();
    assert!(updated_task.last_run.is_some());
}

#[test]
fn test_get_all_tasks() {
    let conn = Connection::open_in_memory().unwrap();
    init_db(&conn).unwrap(); // Pass the in-memory connection

    let task1 = make_task("task1");
    task1.insert(&conn).unwrap();

    let task2 = make_task("task2");
    task2.insert(&conn).unwrap();

    let tasks = get_all_tasks(&conn, None).unwrap();
    assert_eq!(tasks.len(), 2);
}

#[test]
fn test_get_task_logs() {
    let conn = Connection::open_in_memory().unwrap();
    init_db(&conn).unwrap(); // Pass the in-memory connection

    let mut task = make_task("task1");
    task.insert(&conn).unwrap(); // Insert the task before updating logs
    task.start_time = Some(Utc::now());
    task.last_run = Some(Utc::now());
    task.update(&conn).unwrap();

    let logs = get_task_logs(&conn, Some("task1".to_string()), 10).unwrap();
    assert_eq!(logs.len(), 1);
}

#[test]
fn test_reset_command() {
    let conn = Connection::open_in_memory().unwrap();
    init_db(&conn).unwrap();

    // First, add some tasks
    let task1 = make_task("reset_test_1");
    task1.insert(&conn).unwrap();

    let task2 = make_task("reset_test_2");
    task2.insert(&conn).unwrap();

    // Verify tasks were added
    let tasks_before = get_all_tasks(&conn, None).unwrap();
    assert_eq!(tasks_before.len(), 2);

    // Reset the database
    clean_db(&conn).unwrap();

    // Verify the tasks table is now empty
    let tasks_after = get_all_tasks(&conn, None).unwrap();
    assert_eq!(tasks_after.len(), 0);

    // Verify we can still add new tasks after reset
    let task3 = make_task("post_reset_task");
    task3.insert(&conn).unwrap();

    let final_tasks = get_all_tasks(&conn, None).unwrap();
    assert_eq!(final_tasks.len(), 1);
    assert_eq!(final_tasks[0].0, "post_reset_task");
}

#[test]
fn test_delete_command() {
    use lastrun::db::{delete_task, delete_task_logs};

    let conn = Connection::open_in_memory().unwrap();
    init_db(&conn).unwrap();

    // Create a task
    let mut task = make_task("delete_test");
    task.insert(&conn).unwrap();

    // Add a log entry by updating the task with start and end times
    task.start_time = Some(Utc::now());
    task.update(&conn).unwrap(); // This should create a start time

    // Wait a tiny bit to ensure time difference
    std::thread::sleep(std::time::Duration::from_millis(10));

    task.last_run = Some(Utc::now());
    task.update(&conn).unwrap(); // This should create a log entry

    // Verify task and log exist
    let tasks_before = get_all_tasks(&conn, Some("delete_test".to_string())).unwrap();
    let logs_before = get_task_logs(&conn, Some("delete_test".to_string()), 10).unwrap();

    assert_eq!(tasks_before.len(), 1);
    assert_eq!(logs_before.len(), 1);

    // Delete logs first
    let logs_deleted = delete_task_logs(&conn, "delete_test").unwrap();
    assert_eq!(logs_deleted, 1);

    // Verify logs are gone but task remains
    let logs_after = get_task_logs(&conn, Some("delete_test".to_string()), 10).unwrap();
    let tasks_after_log_delete = get_all_tasks(&conn, Some("delete_test".to_string())).unwrap();

    assert_eq!(logs_after.len(), 0);
    assert_eq!(tasks_after_log_delete.len(), 1);

    // Now delete the task
    let task_deleted = delete_task(&conn, "delete_test").unwrap();
    assert_eq!(task_deleted, 1);

    // Verify task is gone
    let tasks_after = get_all_tasks(&conn, Some("delete_test".to_string())).unwrap();
    assert_eq!(tasks_after.len(), 0);

    // Test deleting non-existent task
    let non_existent_deleted = delete_task(&conn, "non_existent_task").unwrap();
    assert_eq!(non_existent_deleted, 0);
}

#[test]
fn test_settings_crud() {
    use lastrun::db::{get_all_settings, get_setting, set_setting};

    let conn = Connection::open_in_memory().unwrap();
    init_db(&conn).unwrap();

    // Setting should not exist initially
    let val = get_setting(&conn, "log_retention").unwrap();
    assert!(val.is_none());

    // Set a value
    set_setting(&conn, "log_retention", "30d").unwrap();
    let val = get_setting(&conn, "log_retention").unwrap();
    assert_eq!(val, Some("30d".to_string()));

    // Update the value
    set_setting(&conn, "log_retention", "60d").unwrap();
    let val = get_setting(&conn, "log_retention").unwrap();
    assert_eq!(val, Some("60d".to_string()));

    // Multiple settings
    set_setting(&conn, "other_key", "some_value").unwrap();
    let all = get_all_settings(&conn).unwrap();
    assert_eq!(all.len(), 2);
    assert!(all.contains(&("log_retention".to_string(), "60d".to_string())));
    assert!(all.contains(&("other_key".to_string(), "some_value".to_string())));
}

#[test]
fn test_log_retention_seconds() {
    use lastrun::db::{get_log_retention_seconds, set_log_retention};

    let conn = Connection::open_in_memory().unwrap();
    init_db(&conn).unwrap();

    // Unset returns None
    let secs = get_log_retention_seconds(&conn).unwrap();
    assert!(secs.is_none());

    // Valid duration
    set_log_retention(&conn, "30d").unwrap();
    let secs = get_log_retention_seconds(&conn).unwrap();
    assert_eq!(secs, Some(30 * 24 * 3600));

    // "off" returns None
    set_log_retention(&conn, "off").unwrap();
    let secs = get_log_retention_seconds(&conn).unwrap();
    assert!(secs.is_none());

    // "0" returns None
    set_log_retention(&conn, "0").unwrap();
    let secs = get_log_retention_seconds(&conn).unwrap();
    assert!(secs.is_none());

    // Invalid duration returns None
    use lastrun::db::set_setting;
    set_setting(&conn, "log_retention", "not_a_duration").unwrap();
    let secs = get_log_retention_seconds(&conn).unwrap();
    assert!(secs.is_none());
}
