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

- Go 1.22 or higher
- [Task](https://taskfile.dev/) (optional, for building from source)

### From Binary

Download the latest binary from the [releases page](https://github.com/yourusername/last-run/releases).

Verify the checksum:
```bash
shasum -a 256 -c lastrun.sha256
```

Make it executable:
```bash
chmod +x lastrun
```

Move it to your PATH:
```bash
sudo mv lastrun /usr/local/bin/
```

### From Source

Clone the repository:
```bash
git clone https://github.com/yourusername/last-run.git
cd last-run
```

Build using Task:
```bash
task build
```

Or build using Go directly:
```bash
go build -o bin/lastrun
```

## Usage

Last Run has two main commands: `update` and `check`.

### Update a Task

Record that a task has been executed:

```bash
lastrun update -id=my-task
```

### Check a Task

Check if a task should be run again based on a time threshold:

```bash
lastrun check -id=my-task -duration=24h
```

The command will exit with code 1 if the task is due to run, making it easy to use in scripts:

```bash
if lastrun check -id=daily-backup -duration=24h; then
  echo "Backup not needed yet"
else
  echo "Running backup..."
  # backup script here
  lastrun update -id=daily-backup
fi
```

### Duration Format

The duration can be specified in Go's duration format:
- `h` for hours (e.g., `24h` for 1 day)
- `m` for minutes (e.g., `30m` for 30 minutes)
- `s` for seconds (e.g., `60s` for 1 minute)

You can combine these units:
- `72h` for 3 days
- `1h30m` for 1 hour and 30 minutes

## Data Storage

Last Run stores task data in a SQLite database located at `~/.tasks/data.db`.
