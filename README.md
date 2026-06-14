# LastRun

A small CLI for tracking when tasks last ran — start/done times, history,
duration thresholds, and an interactive TUI status view.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

## Overview

LastRun is built for cron jobs, scheduled scripts, and any recurring
operation where you want to know *when* it last finished and *whether* it's
due to run again. State is kept in a single SQLite file at a
platform-appropriate location (SQLite is statically linked, so the binary
has no runtime dependencies):

- **macOS:** `~/Library/Application Support/lastrun/data.db`
- **Linux:** `~/.local/share/lastrun/data.db`
- **Windows:** `%APPDATA%\lastrun\data.db`

Override the path with `--db-path` or the `LASTRUN_DB_PATH` environment
variable. See `lastrun --help` for details.

> **Migration from 1.x:** Existing databases at `~/.tasks/data.db` are
> automatically moved to the new location on first run. The old `~/.tasks/`
> directory is removed if empty. No action is needed.

## Features

- Record start and completion times for any task by ID
- `check` exits non-zero when a task is overdue — drop it into cron / shell
  pipelines to gate work
- Interactive `status` TUI with sortable columns, per-task history, and
  duration stats
- `--json` snapshot for scripts and dashboards
- Configurable log retention with automatic cleanup on every `done`/`update`
- Log archival with confirmation prompt
- Zsh tab completion
- `--quiet` flag for non-interactive use

## Installation

### Pre-built binaries

Download a release from the
[GitHub releases page](https://github.com/eveenendaal/last-run/releases).
Builds are published for:

- `aarch64-apple-darwin` (Apple Silicon Mac)
- `x86_64-apple-darwin` (Intel Mac)
- `x86_64-unknown-linux-gnu` (Linux)

Each binary includes a matching `.sha256` checksum file.

### From source

```bash
git clone https://github.com/eveenendaal/last-run.git
cd last-run
cargo build --release
cp target/release/lastrun /usr/local/bin/
```

Or, with [Task](https://taskfile.dev):

```bash
task install     # cargo install --path . --locked
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

| Key            | Action                                       |
|----------------|----------------------------------------------|
| `↑` / `↓`      | Move selection                               |
| `PgUp` / `PgDn`| Page through the task list                   |
| `<` / `>`      | Cycle sort column                            |
| `s`            | Toggle ascending / descending sort           |
| `Enter`        | Drill into per-task history                  |
| `d`            | Delete the selected task (asks to confirm)   |
| `?`            | Toggle the help overlay                      |
| `q`            | Quit                                         |

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

Print a zsh completion script:

```bash
echo 'source <(lastrun completion zsh)' >> ~/.zshrc
```

(Bash, fish, etc. are not currently wired up.)

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

The project uses [Task](https://taskfile.dev) to wrap the common Cargo
invocations:

```bash
task test                    # cargo test --locked
task build                   # release build + SHA256 (native target)
task build TARGET=<triple>   # release build for a specific target triple
task install                 # cargo install --path . --locked
task status                  # run the status TUI against your local DB
task clean                   # cargo clean
```

Run `task test` before committing — that's what CI runs on every PR.

## Project structure

```
last-run/
├── src/
│   ├── main.rs          # Command dispatch
│   ├── lib.rs           # Library root + APP_VERSION
│   ├── cli.rs           # clap definitions, should_run_task()
│   ├── db.rs            # SQLite connection, schema, CRUD
│   ├── model.rs         # Task struct + persistence
│   ├── error.rs         # thiserror-based error type
│   ├── format.rs        # Duration parsing/formatting
│   └── display/
│       ├── mod.rs       # Re-exports + ANSI colour constants
│       ├── json.rs      # JSON status output
│       ├── table.rs     # prettytable log output
│       └── tui.rs       # ratatui interactive status view
├── tests/               # Unit + integration tests
├── examples/            # Example shell scripts
├── docs/                # Architecture notes
├── build.rs             # Injects APP_VERSION from git tag / RELEASE_VERSION
├── Taskfile.yml         # Task runner definitions
└── Cargo.toml
```

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for a deeper architecture
description.

## Contributing

Contributions welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE).

## Author

Eric Veenendaal
