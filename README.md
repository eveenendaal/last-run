# LastRun

A small CLI for tracking when tasks last ran — start/done times, history,
duration thresholds, and an interactive TUI status view.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

## Overview

LastRun is built for cron jobs, scheduled scripts, and any recurring
operation where you want to know *when* it last finished and *whether* it's
due to run again. State is kept in a single SQLite file at a
platform-appropriate location (SQLite is compiled in via the pure-Go
`modernc.org/sqlite` driver, so the binary is statically linked with no
runtime dependencies):

- **macOS:** `~/Library/Application Support/lastrun/data.db`
- **Linux:** `~/.local/share/lastrun/data.db`
- **Windows:** `%LOCALAPPDATA%\lastrun\data.db`

Override the path with `--db-path` or the `LASTRUN_DB_PATH` environment
variable. See `lastrun --help` for details.

## Features

- Record start and completion times for any task by ID
- `check` exits non-zero when a task is overdue — drop it into cron / shell
  pipelines to gate work
- Interactive `status` TUI with sortable columns, per-task history, and
  duration stats
- `--json` snapshot for scripts and dashboards
- Configurable log retention with automatic cleanup on every `done`/`update`
- Log archival with confirmation prompt
- Shell tab completion (bash, zsh, fish, powershell)
- `--quiet` flag for non-interactive use

## Installation

### Pre-built binaries

Download a release from the
[GitHub releases page](https://github.com/eveenendaal/last-run/releases).
Builds are published for:

- `lastrun-darwin-arm64` (Apple Silicon Mac)
- `lastrun-darwin-amd64` (Intel Mac)
- `lastrun-linux-amd64` (Linux x86-64)
- `lastrun-linux-arm64` (Linux ARM64)
- `lastrun-windows-amd64.exe` (Windows x86-64)

Each binary includes a matching `.sha256` checksum file.

### From source

Requires Go (see `go.mod` for the version).

```bash
git clone https://github.com/eveenendaal/last-run.git
cd last-run
go build -o lastrun .
cp lastrun /usr/local/bin/
```

Or, with [Task](https://taskfile.dev):

```bash
task install     # go install .
```

## Quick start

```bash
# Record that a task is starting
lastrun start --id my-task

# ...do the work...

# Record completion (also records elapsed time if you called start first)
lastrun done --id my-task

# Look at the current state of every tracked task
lastrun status

# In a cron job: only run if the last successful run was more than 24h ago
lastrun --quiet check --id my-task --duration 24h || run-the-thing
```

## Commands

### `start` / `done` / `update`

`start` stamps the task with a start time and clears any previous last-run
time. `done` (also aliased as `update`) stamps the last-run time and, if a
start time was recorded, writes an elapsed-time entry to the log.

```bash
lastrun start --id backup
lastrun done  --id backup
```

The short flag `-i` works everywhere `--id` does.

### `check`

Check whether a task is due to run again. Exits **0** if the task ran within
the threshold, **1** if it's overdue (or has never run, or doesn't exist).
The duration accepts `h`, `d`, `w`, and `m` suffixes (hours, days, weeks,
months-of-30-days) — e.g. `24h`, `7d`, `2w`, `3m`.

```bash
lastrun check --id backup --duration 24h
echo $?     # 0 if not yet due, 1 if due
```

The threshold passed to `check` is persisted on the task, so the TUI status
view can colour rows based on what *that* task considers stale.

### `status`

With no flags, opens an interactive TUI showing every tracked task:

```bash
lastrun status
```

Keybindings inside the TUI:

| Key                | Action                                       |
|--------------------|----------------------------------------------|
| `↑` / `↓` (`k`/`j`) | Move selection                               |
| `PgUp` / `PgDn`    | Page through the task list                   |
| `←` / `→` / `Tab`  | Cycle sort column                            |
| `s`                | Toggle ascending / descending sort           |
| `Enter` / `h`      | Drill into per-task history                  |
| `d`                | Delete the selected task (asks to confirm)   |
| `r`                | Refresh now                                  |
| `?`                | Toggle the help overlay                      |
| `q` / `Esc`        | Quit                                         |

For scripts and dashboards, use `--json`:

```bash
lastrun status --json
```

The status view sorts by last-run time by default; override with
`--sort task|status|duration|elapsed|last-run`.

### `logs`

Show the most recent completion log entries (each entry has a task ID, an
end time, and the elapsed time between `start` and `done`):

```bash
lastrun logs                       # 20 most recent across all tasks
lastrun logs --id backup           # filter to one task
lastrun logs --id backup --limit 5 # cap at 5
lastrun logs --limit 0             # no limit
```

### `clear`

Reset just the timing fields on a task without deleting its history:

```bash
lastrun clear --id backup
```

### `delete`

Remove a task and every log entry attached to it:

```bash
lastrun delete --id backup
```

### `archive`

Delete log entries older than a threshold. Defaults to the stored retention
setting (or 30d if none is set). Pass `--older-than` to override.

```bash
lastrun archive                          # uses stored retention (default 30d)
lastrun archive --older-than 7d          # override to 7 days
lastrun archive --older-than 30d --yes   # skip confirmation
lastrun archive --id backup              # only for one task
```

### `set-retention`

Set the log retention period for automatic cleanup. After this is set,
every `done`/`update` call will automatically delete log entries older
than the threshold. Pass `off` (or `0`) to disable auto-cleanup.

```bash
lastrun set-retention 60d                # keep 60 days of logs
lastrun set-retention 2w                 # keep 2 weeks
lastrun set-retention off                # disable auto-cleanup
```

The retention setting can also be changed interactively via `lastrun settings`.

### `settings`

Open an interactive TUI for viewing and editing stored settings (currently
just `log_retention`):

```bash
lastrun settings
```

Keybindings inside the TUI:

| Key            | Action                                  |
|----------------|------------------------------------------|
| `Enter`        | Edit the selected setting (or save while editing) |
| `Esc`          | Cancel the edit, or quit if not editing  |
| `Backspace`    | Delete a character while editing         |
| `?`            | Toggle the help overlay                  |
| `q`            | Quit                                      |

### `reset`

Drop and recreate the `tasks` table — wipes every task but keeps the
`task_log` history. Mostly useful when the schema is in a weird state.

### `completion`

Print a shell completion script (bash, zsh, fish, or powershell):

```bash
echo 'source <(lastrun completion zsh)' >> ~/.zshrc
lastrun completion bash > /etc/bash_completion.d/lastrun
```

Run `lastrun completion --help` for per-shell setup instructions.

### Quiet mode

`--quiet` (`-q`) is a top-level flag — pass it **before** the subcommand
to suppress informational output. Errors and exit codes are unchanged,
which makes the flag safe to use in cron.

```bash
lastrun --quiet start --id backup
lastrun -q check --id backup --duration 24h
```

## Examples

See the [`examples/`](examples/) directory:

- [`basic_usage.sh`](examples/basic_usage.sh) — start / done / status / logs
  walkthrough
- [`cron_integration.sh`](examples/cron_integration.sh) — using `check` to
  gate work and `start`/`done` to record timing in a cron job

## Development

The project uses [Task](https://taskfile.dev) to wrap the common Go
invocations:

```bash
task test                          # go test ./...
task build                         # build dist/lastrun + SHA256 (native target)
GOOS=windows GOARCH=amd64 task build  # cross-compile for a specific target
task install                       # go install .
task status                        # run the status TUI against your local DB
task clean                         # remove dist/
```

Run `task test` before committing — that's what CI runs on every PR.

## Project structure

```
last-run/
├── main.go                  # Entry point: cobra tree, run via fang
├── internal/
│   ├── cli/                 # cobra commands, dispatch, ShouldRunTask()
│   ├── db/                  # SQLite connection, schema, CRUD
│   ├── model/               # Task struct + persistence
│   ├── format/              # Duration parsing/formatting, RFC3339 helpers
│   ├── apperr/              # Error types
│   ├── display/             # JSON status, log table, ANSI colours
│   ├── tui/                 # Bubble Tea interactive status view
│   ├── settings/            # Bubble Tea interactive settings editor
│   ├── tuiutil/             # Shared TUI panels/overlays
│   └── version/             # Release version-bump helper
├── examples/                # Example shell scripts
├── docs/                    # Architecture notes
├── Taskfile.yml             # Task runner definitions
└── go.mod / go.sum
```

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for a deeper architecture
description.

## Contributing

Contributions welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE).

## Author

Eric Veenendaal
