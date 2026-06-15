# Architecture

## Overview

LastRun is a small Go CLI for tracking task execution. It stores state in a
single SQLite database file and exposes both a scripting-friendly command
surface and interactive TUI views. SQLite is provided by the pure-Go,
cgo-free `modernc.org/sqlite` driver, so the released binaries are statically
linked with no runtime dependencies and cross-compile for every target from a
single Linux runner.

## Project structure

```
last-run/
├── main.go                  # Entry point: wires cobra tree, runs via fang
├── internal/
│   ├── cli/                 # cobra commands, dispatch, ShouldRunTask()
│   ├── db/                  # Connection, schema, typed CRUD helpers
│   ├── model/               # Task struct + persistence
│   ├── format/              # Duration parse/format, RFC3339 helpers
│   ├── apperr/              # Sentinel errors + DurationParseError
│   ├── display/             # JSON status, log table, ANSI colour constants
│   ├── tui/                 # Bubble Tea interactive status view
│   ├── settings/            # Bubble Tea interactive settings editor
│   ├── tuiutil/             # Shared TUI primitives (panels, overlays, controls)
│   └── version/             # Release version-bump helper
├── examples/                # Example shell scripts
├── docs/                    # This file
├── Taskfile.yml             # Task runner targets
└── go.mod / go.sum
```

## Core packages

### `cli`
The cobra command tree: `start`, `done`/`update`, `check`, `status`, `logs`,
`archive`, `set-retention`, `clear`, `delete`, `reset`, `settings`. (Shell
completion is provided automatically by cobra's built-in `completion` command,
covering bash/zsh/fish/powershell.) Commands are grouped for a tidy help layout,
which is rendered with styling by [`charmbracelet/fang`](https://github.com/charmbracelet/fang).
`cli` also owns `ShouldRunTask()`, the shared "is this task overdue?" logic used
by both `check` and the TUI's status colouring. The database handle is opened in
a `PersistentPreRunE` hook and closed afterwards.

### `db`
Resolves the database path (`--db-path` flag → `LASTRUN_DB_PATH` env →
`${XDG_DATA_HOME}/lastrun/data.db` via `adrg/xdg`), creates the directory on
demand, and provides typed CRUD helpers over `database/sql`. On first run with
the default path, data is migrated from the old `~/.tasks/data.db` location. The
connection pool is capped at a single connection to avoid SQLite lock
contention. Every query uses parameterized bindings.

### `model`
`Task` is the in-memory representation of a row from the `tasks` table.
`Task.Update()` writes back to `tasks`, and when both `StartTime` and `LastRun`
are set it also appends an entry to `task_log` with the computed elapsed time
in milliseconds.

### `apperr`
Sentinel errors (`ErrMissingTaskID`, `ErrDataDirectoryNotFound`) and the
`DurationParseError` type, matchable via `errors.Is` / `errors.As`.

### `format`
Parses durations like `24h` / `7d` / `2w` / `3m` (months = 30 days), and formats
`time.Duration` values into compact human-readable strings. Timestamps are
persisted as RFC3339 UTC strings.

### `display`
Plain-output renderers driven from `cli` after the `db` layer returns data:

- `json.go` — Serializes `lastrun status --json`: per-task ID, last-run time,
  elapsed time, and computed status (`running` / `due` / `ok` / `unknown`).
- `table.go` — Renders `lastrun logs` as a bordered `lipgloss` table.
- `colors.go` — ANSI colour constants for the printf-style status messages.

### `tui` / `settings` / `tuiutil`
The interactive views, built on [Bubble Tea](https://github.com/charmbracelet/bubbletea)
and [Lip Gloss](https://github.com/charmbracelet/lipgloss):

- `tui` — The `lastrun status` view: sortable task table, per-task history
  drill-down with stats, delete-confirmation popups, and a `?` help overlay.
  It refreshes every 250 ms so elapsed counters tick live.
- `settings` — The `lastrun settings` editor for viewing/editing key-value
  settings (currently `log_retention`), routing saves through the validating
  setter.
- `tuiutil` — Shared rendering primitives: titled rounded panels, the bottom
  controls bar, the help modal, and a centered overlay compositor.

## Storage

Three tables, all created idempotently in `db.InitDB()`:

```sql
CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    last_run TEXT,        -- RFC3339 timestamp, NULL if never completed
    start_time TEXT,      -- RFC3339 timestamp, NULL if not currently running
    duration INTEGER      -- Most recently used `check --duration`, in seconds
);

CREATE TABLE task_log (
    id TEXT,
    end_time TEXT,
    elapsed_time INTEGER, -- Milliseconds between start and done
    PRIMARY KEY (id, end_time)
);

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
```

The `settings` table is a generic key-value store; the only key currently in use
is `log_retention` (a duration string like `30d`, or `off` to disable cleanup).

Schema migrations are intentionally minimal: `InitDB()` runs the
`CREATE TABLE IF NOT EXISTS` statements, then attempts a best-effort
`ALTER TABLE tasks ADD COLUMN duration INTEGER`, ignoring the error if the
column already exists. There is no migration version table — the schema is small
and changes are additive. This is the same schema the original Rust version
used, so existing databases work unchanged.

## Data flow

1. `main.go` builds the cobra tree and executes it through `fang`.
2. `PersistentPreRunE` opens the DB and runs `InitDB()`.
3. The matched command calls into `model` (typed task ops) or `db` (bulk reads,
   archival, deletions).
4. After every `done`/`update`, `autoArchive()` reads the `log_retention` setting
   and deletes log entries older than the threshold (defaulting to 30 days).
5. The result is handed to the appropriate renderer:
   - Plain printf for `start`/`done`/`check`/`clear`/`delete`/`reset`.
   - `display.PrintTaskLogs` for `logs`.
   - `display.PrintTaskStatusJSON` for `status --json`.
   - `tui.RunTUI` for `status` (default).
   - `settings.RunSettingsTUI` for `settings`.

## Versioning

The release version is injected at build time via
`-ldflags "-X main.version=<version>"` and exposed through `--version`. The CI
`build` job sets it from the next git tag (`RELEASE_VERSION`); local builds fall
back to `git describe --tags` (or `dev`). That means the binary always reports
the release version, even when built from a checkout where no tag is present.

## Dependencies

- **modernc.org/sqlite** — pure-Go SQLite, statically linked, no cgo
- **spf13/cobra** + **charmbracelet/fang** — CLI parsing and styled help
- **charmbracelet/bubbletea** + **lipgloss** — TUI rendering and input
- **adrg/xdg** — locating the platform data directory for the default DB path
