mod helper;
use chrono::Utc;
use helper::make_task;
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

    let fetched_task = Task::select(&conn, "test_task", true).unwrap();
    assert_eq!(fetched_task.id, "test_task");

    task.last_run = Some(Utc::now());
    task.update(&conn).unwrap();

    let updated_task = Task::select(&conn, "test_task", true).unwrap();
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
