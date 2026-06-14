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
2. `version` job (Ubuntu) computes the next patch version from the latest
   git tag, sed-bumps `Cargo.toml` *locally in the runner*, refreshes
   `Cargo.lock`, and uploads the patched manifests as an artifact.
3. `build` job matrix builds on native runners for each target OS:
   - **macOS** (Apple Silicon): `aarch64-apple-darwin` + cross-compiled
     `x86_64-apple-darwin`
   - **Linux**: `x86_64-unknown-linux-gnu`
   
   Each runner downloads the patched manifests, builds with `--locked`, and
   uploads its binaries as an artifact. The version from `build.rs` is fed in
   via the `RELEASE_VERSION` env var.
4. `create-release` job (Ubuntu) downloads all artifacts, writes a `VERSION`
   file, and calls `softprops/action-gh-release` to create the release tag
   (`v*`) and upload all binaries.
5. Only the 3 most recent releases are kept.

To add more targets, add an entry to the build matrix and include its artifact
directory in the `create-release` job's `files:` list.

## Conventions

- Version lives in git tags (`v*`), bumped automatically (patch) on release.
  `build.rs` reads the next tag (or `RELEASE_VERSION`) at compile time and
  bakes it into the binary.
- Dependabot is configured in `.github/dependabot.yml` (monthly Cargo +
  GitHub Actions updates, assigned to `eveenendaal`). PRs run
  `.github/workflows/test.yml`; there is no auto-merge workflow — merges
  are reviewed manually.
- Always run `task test` before committing.
