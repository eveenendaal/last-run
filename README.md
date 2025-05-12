# Last Run

A simple command-line tool to track when tasks were last executed and determine if they should be run again based on a specified time threshold.

## Overview

Last Run helps you manage recurring tasks by tracking when they were last executed. It can tell you if a task is due to run again based on a specified duration since its last execution. This is particularly useful for scripts that should only run at certain intervals.

## Features

## Usage

Last Run has three main commands: `start`, `update` (or `done`), and `check`.

### Start a Task

Record the start time of a task:

```bash
lastrun start --id my-task
```

Or using the short option:
```bash
lastrun start -i my-task
```

### Update a Task

Record that a task has been completed. If the task was started using the `start` command, the elapsed time will be calculated and displayed:

```bash
lastrun update --id my-task
```

Or using the short option:
```bash
lastrun update -i my-task
```

You can also use the `done` synonym for `update`:

```bash
lastrun done --id my-task
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

