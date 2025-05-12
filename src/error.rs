use thiserror::Error;

#[derive(Error, Debug)]
pub enum AppError {
    #[error("Database error: {0}")]
    Database(#[from] rusqlite::Error),

    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),

    #[error("Date parsing error: {0}")]
    DateParse(#[from] chrono::ParseError),

    #[error("Duration parsing error: {0}")]
    DurationParse(String),

    #[error("Task ID is required")]
    MissingTaskId,

    #[error("Home directory not found")]
    HomeDirectoryNotFound,
}

pub type AppResult<T> = Result<T, AppError>;
