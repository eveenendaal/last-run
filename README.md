# Last Run

A simple command-line tool to track when tasks were last executed and determine if they should be run again based on a specified time threshold.

## Overview

Last Run helps you manage recurring tasks by tracking when they were last executed. It can tell you if a task is due to run again based on a specified duration since its last execution. This is particularly useful for scripts that should only run at certain intervals.

## Features

## Usage

Last Run has several commands for tracking and monitoring tasks: `start`, `update` (or `done`), `check`, `status`, `logs`, and `reset`.

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

### View Task Status

Display the current status of all tasks:

```bash
lastrun status
```

To filter by a specific task ID:

```bash
lastrun status --id my-task
```

Or using the short option:
```bash
lastrun status -i my-task
```

### View Task Logs

Display execution logs for all tasks:

```bash
lastrun logs
```

To filter by a specific task ID:

```bash
lastrun logs --id my-task
```

To change the number of logs displayed (default is 20):

```bash
lastrun logs --limit 50
```

Or using short options:
```bash
lastrun logs -i my-task -l 50
```

### Reset Database

Reset the tasks database, rebuilding the tables:

```bash
lastrun reset
```

### Delete Task Records

Delete a task and all its log entries:

```bash
lastrun delete --id my-task
```

Or using the short option:
```bash
lastrun delete -i my-task
```

### Quiet Mode

Add the `-q` or `--quiet` flag to suppress output messages:

```bash
lastrun start --id my-task -q
```
`