use lastrun::error::{AppError, AppResult};
use std::io;

#[test]
fn test_error_conversion() {
    // Test IO error conversion
    let io_error = io::Error::new(io::ErrorKind::NotFound, "File not found");
    let app_error: AppError = io_error.into();
    match app_error {
        AppError::Io(_) => (),
        _ => panic!("Expected IO error variant"),
    }

    // Test duration parsing error
    let duration_error = AppError::DurationParse("Invalid duration format".to_string());
    let error_msg = format!("{}", duration_error);
    assert!(error_msg.contains("Duration parsing error"));

    // Test missing task ID error
    let missing_id_error = AppError::MissingTaskId;
    let error_msg = format!("{}", missing_id_error);
    assert_eq!(error_msg, "Task ID is required");

    // Test home directory not found error
    let home_dir_error = AppError::HomeDirectoryNotFound;
    let error_msg = format!("{}", home_dir_error);
    assert_eq!(error_msg, "Home directory not found");
}

#[test]
fn test_app_result() {
    // Test successful result
    let success: AppResult<i32> = Ok(42);
    assert_eq!(success.unwrap(), 42);

    // Test error result
    let error: AppResult<()> = Err(AppError::MissingTaskId);
    assert!(error.is_err());

    // Test error chaining with map_err
    let io_error = io::Error::new(io::ErrorKind::PermissionDenied, "Permission denied");
    let result: Result<(), io::Error> = Err(io_error);
    let app_result: AppResult<()> = result.map_err(AppError::from);

    match app_result {
        Err(AppError::Io(e)) => assert_eq!(e.kind(), io::ErrorKind::PermissionDenied),
        _ => panic!("Expected IO error variant"),
    }
}
