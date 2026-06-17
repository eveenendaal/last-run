# LastRun

A Go CLI (`lastrun`) that tracks when tasks were last run — start/complete/fail
times, history, and a TUI status view. Storage is SQLite (via the pure-Go
`modernc.org/sqlite` driver). This is a **public, open-source** project. Binaries
are available on the
[GitHub releases page](https://github.com/eveenendaal/last-run/releases).

## Working Effectively

Uses [Task](https://taskfile.dev). The toolchain is just Go + Task.

```bash
task test                       # go test ./...
task build                      # build dist/lastrun + lastrun.sha256 (native)
GOOS=windows GOARCH=amd64 task build   # cross-compile for another target
task status                     # run the status TUI
task help                       # styled help output
task clean                      # remove dist/
```

Source lives in `main.go` plus `internal/` packages: `cli` (cobra commands +
`ShouldRunTask`), `config` (per-user JSON config file), `db` (SQLite schema +
CRUD), `model` (`Task` persistence), `format` (duration/time helpers), `apperr`
(errors), `display` (JSON + log table + ANSI colors), `tui` (Bubble Tea status
view), `settings` (Bubble Tea settings editor with db location, import/export),
`tuiutil` (shared TUI panels/overlays), and `version` (release bump helper).
Architecture notes in `docs/ARCHITECTURE.md`.

### Pure-Go SQLite (no cgo)
The `modernc.org/sqlite` driver is a cgo-free, pure-Go SQLite. Binaries are
statically linked with no `libsqlite3` runtime dependency, and **every release
target — Linux, macOS (both arches), and Windows — cross-compiles from a single
Linux runner** with `CGO_ENABLED=0`. The on-disk database format is standard
SQLite, so existing `data.db` files keep working unchanged.

## Libraries

- **CLI:** `spf13/cobra` wrapped with `charmbracelet/fang` for styled, grouped
  help output.
- **TUI:** `charmbracelet/bubbletea` + `lipgloss` (status + settings views).
- **SQLite:** `modernc.org/sqlite` via `database/sql`.
- **Data dir:** `github.com/adrg/xdg` for the default DB location.

## Releases & multi-target builds

Releases are produced by `.github/workflows/build.yml` on push to `master`:

1. `test` job (Ubuntu) runs `task test`.
2. `version` job computes the next patch version from the latest git tag (`v*`).
3. `build` job (Ubuntu) cross-compiles every target in one loop with
   `CGO_ENABLED=0`, injecting the version via `-ldflags "-X main.version=..."`,
   and writes a `.sha256` per binary. Targets: `linux/amd64`, `linux/arm64`,
   `darwin/amd64`, `darwin/arm64`, `windows/amd64`.
4. `create-release` downloads the artifacts, writes a `VERSION` file, and calls
   `softprops/action-gh-release` to create the `v*` tag and upload binaries.
5. Only the 3 most recent releases are kept.

To add a target, add it to the `targets` list in the `build` job.

## Conventions

- Version lives in git tags (`v*`), bumped automatically (patch) on release and
  baked into the binary at build time via `-ldflags`. Local builds fall back to
  `git describe` (or `dev`).
- Dependabot is configured in `.github/dependabot.yml` (monthly Go modules +
  GitHub Actions updates, assigned to `eveenendaal`). PRs run
  `.github/workflows/test.yml`; merges are reviewed manually.
- Always run `task test` before committing.
