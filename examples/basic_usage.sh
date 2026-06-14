#!/bin/bash
# Example: Basic usage of lastrun

# Start tracking a backup task
lastrun start --id backup

# Do some work...
echo "Performing backup..."
sleep 2

# Mark the task as done (records last-run time + elapsed time)
lastrun done --id backup

# View current status of all tasks (interactive TUI; press q to quit)
lastrun status

# Or grab a JSON snapshot for scripts
lastrun status --json

# View execution logs for this task
lastrun logs --id backup
