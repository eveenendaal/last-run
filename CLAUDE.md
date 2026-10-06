# LastRun

A Go CLI (`lastrun`) that tracks when tasks were last run — start/complete/fail
times, history, and a TUI status view. Storage is SQLite (via the pure-Go
`modernc.org/sqlite` driver). This is a **public, open-source** project. Binaries
are available on the
[GitHub releases page](https://github.com/eveenendaal/last-run/releases).

## Working Effectively

Uses a small `Makefile`. The toolchain is just Go + make.

```bash
make test                       # go test ./...
make build                      # build dist/lastrun + lastrun.sha256 (native)
GOOS=windows GOARCH=amd64 make build   # cross-compile for another target
make install                    # build into $GOBIN (or $GOPATH/bin)
make clean                      # remove dist/
go run ./cmd/lastrun status     # run the status TUI
```

Source lives in `cmd/lastrun/main.go` (with an identical `main.go` at the
repository root) plus `internal/` packages: `cli` (cobra commands +
`ShouldRunTask`), `config` (per-user JSON config file), `db` (SQLite schema,
CRUD, and the shared `TaskStatus.Status`/`Elapsed` logic), `model` (`Task`
persistence), `format` (duration/time helpers), `apperr` (errors), `display`
(JSON + log table + ANSI colors), `tui` (Bubble Tea status view), `settings`
(Bubble Tea settings editor with db location, import/export), and `tuiutil`
(shared TUI panels/overlays). Architecture notes in `docs/ARCHITECTURE.md`.

CLI handlers print through `appContext.printf` (respects `--quiet`, writes to
the command's output) so `internal/cli/cli_test.go` can drive the real command
tree via `runCLI`. `check` signals "due" by returning `ErrTaskDue`, never
`os.Exit`.

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

1. `test` job (Ubuntu) runs `make test`.
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
  `.github/workflows/test.yml`, whose `test` job is a required check on
  `master` (set in terraform-base), so nothing merges before it passes.
- Always run `make test` before committing.
