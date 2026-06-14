# Architecture

## Overview

LastRun is a small Rust CLI for tracking task execution. It stores state in
a single SQLite database file and exposes both a scripting-friendly command
surface and an interactive TUI status view. SQLite is statically linked
(`rusqlite`'s `bundled` feature), so the released binaries have no runtime
dependencies.

## Project structure

```
last-run/
├── src/
│   ├── main.rs          # Command dispatch
│   ├── lib.rs           # Library root, re-exports, APP_VERSION
│   ├── cli.rs           # clap definitions, should_run_task()
│   ├── db.rs            # Connection, schema, CRUD
│   ├── model.rs         # Task struct + persistence
│   ├── error.rs         # thiserror error enum
│   ├── format.rs        # Duration parse/format helpers
│   └── display/
│       ├── mod.rs       # Re-exports, ANSI colour constants
│       ├── json.rs      # JSON status output
│       ├── table.rs     # prettytable log output
│       └── tui.rs       # ratatui interactive status view
├── tests/               # Unit + integration tests
├── examples/            # Example shell scripts
├── docs/                # This file
├── build.rs             # Injects APP_VERSION at compile time
├── Taskfile.yml         # Task runner targets
└── Cargo.toml
```

## Core modules

### `cli.rs`
clap-derived CLI: `start`, `done`/`update`, `check`, `logs`, `status`,
`reset`, `delete`, `clear`, `archive`, `set-retention`, `completion`. Also home to
`should_run_task()`, the shared "is this task overdue?" logic used by both
the `check` command and the TUI's status colouring.

### `db.rs`
Resolves the database path (using `--db-path` CLI flag, `LASTRUN_DB_PATH`
env var, or `dirs::data_dir()` as default), creates the directory on demand,
and provides typed CRUD helpers. On first run with the new default path,
data is automatically migrated from the old `~/.tasks/data.db` location.
Every query goes through parameterized bindings — no string concatenation
of user input into SQL.

### `model.rs`
`Task` is the in-memory representation of a row from the `tasks` table.
`Task::update()` writes back to `tasks`, and when both `start_time` and
`last_run` are set it also appends an entry to `task_log` with the
computed elapsed time.

### `error.rs`
`AppError` (via `thiserror`) and the `AppResult<T>` alias used throughout
the binary.

### `format.rs`
Parses durations like `24h` / `7d` / `2w` / `3m`, and formats `chrono`
durations into compact human-readable strings.

### `display/`
Three rendering surfaces, all driven from `main.rs` after the DB layer
returns data:

- `display/json.rs` — Serializes `lastrun status --json` output: per-task
  ID, last-run time, elapsed time, computed status (`running` /
  `due` / `ok` / `unknown`).
- `display/table.rs` — Renders `lastrun logs` as a `prettytable` (task ID,
  completion time, duration).
- `display/tui.rs` — The interactive `lastrun status` TUI built on
  `ratatui` + `crossterm`. Handles event loop, sort cycling, history
  drill-down, delete confirmation, and the `?` help overlay. Refresh
  cadence is 250 ms so elapsed counters tick live.

`display/mod.rs` re-exports the entry points and shared ANSI colour
constants used by `main.rs`'s plain printf-style output.

## Storage

Three tables, all created in `db::init_db()`:

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

The `settings` table is a generic key-value store. The only key currently
in use is `log_retention`, which holds a duration string (e.g. `30d`, `2w`)
or `off` to disable automatic log cleanup.

Schema migrations are intentionally minimal: `init_db()` always runs the
`CREATE TABLE IF NOT EXISTS` statements, then attempts an `ALTER TABLE
tasks ADD COLUMN duration INTEGER` and silently ignores the error if the
column already exists. There's no migration version table — the schema is
small and changes are additive.

## Data flow

1. `main.rs` parses CLI args, opens the DB, runs `init_db()`.
2. The matched subcommand calls into `model.rs` (typed task ops) or
   directly into `db.rs` (bulk reads, archival, deletions).
3. After every `done`/`update`, `auto_archive()` reads the `log_retention`
   setting and deletes log entries older than the threshold (defaulting to
   30 days if not configured).
4. The result is handed to whichever renderer the command needs:
   - Plain `println!` for `start`/`done`/`check`/`clear`/`delete`/`reset`.
   - `display::print_task_logs` for `logs`.
   - `display::print_task_status_json` for `status --json`.
   - `display::run_tui` for `status` (default).

## Versioning

`build.rs` runs at compile time and sets the `APP_VERSION` environment
variable. The CLI exposes that as the `--version` output via
`#[command(version = env!("APP_VERSION"))]` in `cli.rs`. The resolution
order is:

1. `RELEASE_VERSION` env var (the CI workflow sets this from the next
   git tag),
2. `git describe --tags --abbrev=0`,
3. `CARGO_PKG_VERSION` (i.e. whatever is currently in `Cargo.toml`).

That means the binary always reports the *release* version, even when
built from a checkout where `Cargo.toml` lags behind the latest tag.

## Dependencies

- **rusqlite** (`bundled`) — SQLite, statically linked
- **chrono** — timestamps
- **clap** + **clap_complete** — CLI parsing and shell completion
- **dirs** — locating the platform data directory for the default DB path
- **thiserror** — error type derivation
- **prettytable-rs** — log table rendering
- **serde_json** — JSON status output
- **ratatui** + **crossterm** — TUI rendering and terminal input
