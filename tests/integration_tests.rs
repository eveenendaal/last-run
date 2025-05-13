use chrono::{Duration, Utc};
use lastrun::cli::should_run_task;
use lastrun::db::{get_all_tasks, get_task_logs, init_db, delete_task, delete_task_logs};
use lastrun::model::Task;
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
    let mut task = Task::new(task_id.clone());
    task.insert(&conn).unwrap();
    
    // 2. Verify task exists with no run time
    let tasks = get_all_tasks(&conn, Some(task_id.clone())).unwrap();
    assert_eq!(tasks.len(), 1);
    assert_eq!(tasks[0].0, task_id);
    assert!(tasks[0].1.is_none()); // No last_run yet
    
    // 3. Start the task
    task.start_time = Some(Utc::now());
    task.update(&conn).unwrap();
    
    // 4. Wait a bit and complete the task
    sleep(StdDuration::from_millis(100));
    task.last_run = Some(Utc::now());
    task.update(&conn).unwrap();
    
    // 5. Verify the task has been updated
    let updated_tasks = get_all_tasks(&conn, Some(task_id.clone())).unwrap();
    assert!(updated_tasks[0].1.is_some()); // Has last_run now
    assert!(updated_tasks[0].2.is_none()); // start_time should be reset
    
    // 6. Verify a log entry was created
    let logs = get_task_logs(&conn, Some(task_id.clone()), 10).unwrap();
    assert_eq!(logs.len(), 1);
    assert_eq!(logs[0].0, task_id);
    assert!(logs[0].2 > 0); // Should have some elapsed time
    
    // 7. Test should_run_task logic
    let last_run = updated_tasks[0].1.unwrap();
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
        let task = Task::new(id.to_string());
        task.insert(&conn).unwrap();
    }
    
    // 2. Verify all tasks were created
    let all_tasks = get_all_tasks(&conn, None).unwrap();
    assert_eq!(all_tasks.len(), 3);
    
    // 3. Update tasks with different times
    let mut daily = Task::new("daily_task".to_string());
    daily.last_run = Some(Utc::now() - Duration::hours(12));
    daily.update(&conn).unwrap();
    
    let mut weekly = Task::new("weekly_task".to_string());
    weekly.last_run = Some(Utc::now() - Duration::days(3));
    weekly.update(&conn).unwrap();
    
    let mut monthly = Task::new("monthly_task".to_string());
    monthly.last_run = Some(Utc::now() - Duration::days(15));
    monthly.update(&conn).unwrap();
    
    // 4. Check task status against different thresholds
    let daily_status = get_all_tasks(&conn, Some("daily_task".to_string())).unwrap();
    let daily_last_run = daily_status[0].1.unwrap();
    let (daily_should_run, _) = should_run_task(daily_last_run, Duration::hours(24));
    assert_eq!(daily_should_run, false); // Not due yet (12h < 24h threshold)
    
    let weekly_status = get_all_tasks(&conn, Some("weekly_task".to_string())).unwrap();
    let weekly_last_run = weekly_status[0].1.unwrap();
    let (weekly_should_run, _) = should_run_task(weekly_last_run, Duration::days(7));
    assert_eq!(weekly_should_run, false); // Not due yet (3d < 7d threshold)
    
    let monthly_status = get_all_tasks(&conn, Some("monthly_task".to_string())).unwrap();
    let monthly_last_run = monthly_status[0].1.unwrap();
    let (monthly_should_run, _) = should_run_task(monthly_last_run, Duration::days(14));
    assert_eq!(monthly_should_run, true); // Due (15d > 14d threshold)
    
    // 5. Delete all tasks
    for id in task_ids.iter() {
        delete_task_logs(&conn, id).unwrap();
        delete_task(&conn, id).unwrap();
    }
    
    // 6. Verify all tasks are gone
    let final_tasks = get_all_tasks(&conn, None).unwrap();
    assert_eq!(final_tasks.len(), 0);
}
