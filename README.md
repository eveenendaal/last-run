# Last Run

A simple command-line tool to track when tasks were last executed and determine if they should be run again based on a specified time threshold.

## Overview

Last Run helps you manage recurring tasks by tracking when they were last executed. It can tell you if a task is due to run again based on a specified duration since its last execution. This is particularly useful for scripts that should only run at certain intervals.

## Features

- Track the last execution time of any task with a unique ID
- Check if a task should be run again based on a configurable time threshold
- Simple command-line interface
- Persistent storage using SQLite database
- Human-readable time formatting

## Installation

### Prerequisites

- Rust and Cargo (https://rustup.rs/)

### From Source

Clone the repository:
```bash
git clone https://github.com/eveenendaal/last-run.git
cd last-run
```

Build using Cargo:
```bash
cargo build --release
```

The compiled binary will be available at `target/release/lastrun`.

Move it to your PATH:
```bash
sudo mv target/release/lastrun /usr/local/bin/
```

## Usage

Last Run has two main commands: `update` and `check`.

### Update a Task

Record that a task has been executed:

```bash
lastrun update --id my-task
```

Or using the short option:
```bash
lastrun update -i my-task
```

### Check a Task

Check if a task should be run again based on a time threshold:

```bash
lastrun check --id my-task --duration 24h
```

Or using short options:
```bash
lastrun check -i my-task -d 24h
```

The command will exit with code 1 if the task is due to run, making it easy to use in scripts:

```bash
if lastrun check -i daily-backup -d 24h; then
  echo "Backup not needed yet"
else
  echo "Running backup..."
  # backup script here
  lastrun update -i daily-backup
fi
```

### Quiet Mode

Add the `-q` or `--quiet` flag to suppress output messages:

```bash
lastrun -q update -i my-task
lastrun -q check -i my-task -d 24h
```

### Duration Format

The duration can be specified in the following format:
- `h` for hours (e.g., `24h` for 1 day)
- `d` for days (e.g., `7d` for 1 week)

## Data Storage

Last Run stores task data in a SQLite database located at `~/.tasks/data.db`.
