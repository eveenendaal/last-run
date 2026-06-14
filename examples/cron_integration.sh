#!/bin/bash
# Example: Using lastrun in a cron job
#
# Two patterns are shown:
#
#   1. Gate the job on `lastrun check` so the work only runs if it's been
#      long enough since the last successful run. `check` exits 1 when the
#      task is due (or has never run) — perfect for `|| run-the-thing`.
#
#   2. Record start/done around the actual work so `lastrun status` /
#      `lastrun logs` show timing history. There is no `fail` command:
#      a `start` without a matching `done` leaves the task in the running
#      state, which is what `lastrun status` flags as stale.
#
# Note: `--quiet` is a top-level flag and must come BEFORE the subcommand.

TASK_ID="daily-backup"

# Pattern 1: only run if the task hasn't completed in the last 24h
if lastrun --quiet check --id "$TASK_ID" --duration 24h; then
    echo "Backup ran recently — nothing to do."
    exit 0
fi

# Pattern 2: record start, run the work, record done on success
lastrun --quiet start --id "$TASK_ID"

if /usr/local/bin/backup-script.sh; then
    lastrun --quiet done --id "$TASK_ID"
    exit 0
else
    # Leave the task in "started" state so `lastrun status` shows it as
    # unfinished. Optionally clear it with `lastrun clear --id "$TASK_ID"`.
    echo "Backup failed — leaving task in started state for visibility." >&2
    exit 1
fi
