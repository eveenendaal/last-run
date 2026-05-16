pub mod json;
pub mod table;
pub mod tui;

pub use json::print_task_status_json;
pub use table::print_task_logs;
pub use tui::{run_tui, SortCol};

// ANSI color constants
pub const BOLD: &str = "\x1b[1m";
pub const RESET: &str = "\x1b[0m";
pub const GREEN: &str = "\x1b[32m";
pub const RED: &str = "\x1b[31m";
pub const WHITE: &str = "\x1b[97m";
