#!/bin/bash
# Example: Basic usage of lastrun

# Start tracking a backup task
lastrun start --id backup

# Do some work...
echo "Performing backup..."
sleep 2

# Complete the backup task
lastrun complete --id backup

# Check when it was last run
lastrun list

# View logs
lastrun logs --id backup
