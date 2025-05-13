use chrono::Utc;
use lastrun::db::{get_all_tasks, get_task_logs, init_db};
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

    let mut task = Task::new("test_task".to_string());
    task.insert(&conn).unwrap();

    let fetched_task = task.select(&conn, true).unwrap().unwrap();
    assert_eq!(fetched_task.id, "test_task");

    task.last_run = Some(Utc::now());
    task.update(&conn).unwrap();

    let updated_task = task.select(&conn, true).unwrap().unwrap();
    assert!(updated_task.last_run.is_some());
}

#[test]
fn test_get_all_tasks() {
    let conn = Connection::open_in_memory().unwrap();
    init_db(&conn).unwrap(); // Pass the in-memory connection

    let task1 = Task::new("task1".to_string());
    task1.insert(&conn).unwrap();

    let task2 = Task::new("task2".to_string());
    task2.insert(&conn).unwrap();

    let tasks = get_all_tasks(&conn, None).unwrap();
    assert_eq!(tasks.len(), 2);
}

#[test]
fn test_get_task_logs() {
    let conn = Connection::open_in_memory().unwrap();
    init_db(&conn).unwrap(); // Pass the in-memory connection

    let mut task = Task::new("task1".to_string());
    task.insert(&conn).unwrap(); // Insert the task before updating logs
    task.start_time = Some(Utc::now());
    task.last_run = Some(Utc::now());
    task.update(&conn).unwrap();

    let logs = get_task_logs(&conn, Some("task1".to_string()), 10).unwrap();
    assert_eq!(logs.len(), 1);
}
