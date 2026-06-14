# LastRun

A Rust CLI (`lastrun`) that tracks when tasks were last run — start/complete/fail
times, history, and a TUI status view. Storage is SQLite (via `rusqlite`).
This is a **public, open-source** project. Binaries are available on the
[GitHub releases page](https://github.com/eveenendaal/last-run/releases).

## Working Effectively

Uses [Task](https://taskfile.dev). The whole toolchain is just Rust + Task.

```bash
task test                       # cargo test --locked
task build                      # release build + lastrun.sha256 (native target)
task build TARGET=<triple>      # release build for a specific target triple
task status                     # run the status TUI
task clean                      # cargo clean
```

Source lives in `src/` (`main.rs`, `cli.rs`, `db.rs`, `model.rs`, `format.rs`,
`display/`). Architecture notes in `docs/ARCHITECTURE.md`.

### SQLite is bundled
`rusqlite` is configured with the `bundled` feature (`Cargo.toml`), so SQLite is
compiled into the binary. The result is self-contained — no `libsqlite3` runtime
dependency, which keeps the release binaries portable. Enabling/disabling this feature changes `Cargo.lock`; CI runs
`cargo update --workspace` before building, so keep the committed lockfile in
sync if you build locally with `--locked`.

## Releases & multi-target builds

Releases are produced by `.github/workflows/build.yml` on push to `master`:

1. `test` job (Ubuntu) runs `task test`.
2. `build` job (macOS, Apple Silicon) computes the next patch version from the
   latest git tag, updates `Cargo.toml` locally, refreshes `Cargo.lock`, then
   builds **both macOS targets** — `aarch64-apple-darwin` and
   `x86_64-apple-darwin` — using `task build TARGET=...`. (Apple Silicon runners
   cross-compile the x86_64 target.) No commit is pushed back to `master`.
3. `softprops/action-gh-release` creates the release tag (`v*`) and uploads
   the binaries. The `VERSION` file is generated in the runner and uploaded as
   a release asset.
4. Only the 3 most recent releases are kept.

To add more targets (e.g. Linux), extend the `TARGETS` env in the `build` job and
the `files:` list in the Create Release step. Linux/Windows targets need their own
runners or a cross-compilation setup (e.g. `cross`), since the matrix currently
relies on a macOS runner.

## Conventions

- Version lives in git tags (`v*`), bumped automatically (patch) on release.
- Dependabot PRs are auto-merged (`.github/workflows/auto-merge.yml`); PRs run
  `.github/workflows/test.yml`.
- Always run `task test` before committing.
