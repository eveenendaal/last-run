# Architecture

## Overview

LastRun is a CLI utility built in Rust that helps track when tasks were last executed. It uses SQLite for persistent storage and provides a clean command-line interface.

## Project Structure

```
LastRun/
├── src/              # Source code
│   ├── main.rs       # Entry point
│   ├── lib.rs        # Library root
│   ├── cli.rs        # Command-line interface definitions
│   ├── db.rs         # Database operations
│   ├── model.rs      # Data models
│   ├── error.rs      # Error handling
│   ├── format.rs     # Output formatting
│   └── display.rs    # Display logic
├── tests/            # Integration and unit tests
├── docs/             # Documentation
└── Cargo.toml        # Dependencies and metadata
```

## Core Components

### CLI Module (`cli.rs`)
Defines the command-line interface using the `clap` crate with derive macros.

### Database Module (`db.rs`)
Handles SQLite database operations including:
- Task tracking
- Log entry management
- Database initialization and migrations

### Model Module (`model.rs`)
Contains data structures representing:
- Tasks
- Log entries
- Timestamps

### Error Module (`error.rs`)
Centralized error handling using `thiserror`.

### Format Module (`format.rs`)
Handles different output formats (JSON, table, etc.).

### Display Module (`display.rs`)
Pretty-printing and console output logic.

## Data Flow

1. User executes a command via CLI
2. CLI module parses arguments
3. Database module is called to read/write data
4. Results are formatted via format/display modules
5. Output is presented to the user

## Dependencies

- **rusqlite**: SQLite database interface
- **chrono**: Date and time handling
- **clap**: Command-line argument parsing
- **dirs**: Cross-platform directory paths
- **thiserror**: Error handling
- **prettytable-rs**: Table formatting
- **serde_json**: JSON serialization
- **clap_complete**: Shell completion generation
