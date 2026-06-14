#!/bin/bash
# Example: Using lastrun in a cron job

# This script demonstrates how to use lastrun to track cron job executions

TASK_ID="daily-backup"

# Start the task
lastrun start --id "$TASK_ID" --quiet

# Run your actual backup command
if /usr/local/bin/backup-script.sh; then
    # Mark as complete on success
    lastrun complete --id "$TASK_ID" --quiet
else
    # Mark as failed
    lastrun fail --id "$TASK_ID" --quiet
    exit 1
fi
