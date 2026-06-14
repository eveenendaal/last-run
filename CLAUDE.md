# LastRun

A Rust CLI (`lastrun`) that tracks when tasks were last run — start/complete/fail
times, history, and a TUI status view. Storage is SQLite (via `rusqlite`).
This is a **public, open-source** project distributed through **homebrew-core**
(`brew install lastrun`).

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
dependency, which keeps the release binaries portable and the Homebrew formula
dependency-free. Enabling/disabling this feature changes `Cargo.lock`; CI runs
`cargo update --workspace` before building, so keep the committed lockfile in
sync if you build locally with `--locked`.

## Releases & multi-target builds

Releases are produced by `.github/workflows/build.yml` on push to `master`:

1. `test` job (Ubuntu) runs `task test`.
2. `build` job (macOS, Apple Silicon) bumps the version
   (`eveenendaal/github-actions/actions/rust-version-upgrade`, tag prefix `v`,
   patch bump), refreshes `Cargo.lock`, then builds **both macOS targets** —
   `aarch64-apple-darwin` and `x86_64-apple-darwin` — using
   `task build TARGET=...`. (Apple Silicon runners cross-compile the x86_64
   target.)
3. Each binary is uploaded to the GitHub release as
   `lastrun-<target>` plus a `lastrun-<target>.sha256`, alongside `VERSION`.
4. Only the 3 most recent releases are kept.

To add more targets (e.g. Linux), extend the `TARGETS` env in the `build` job and
the `files:` list in the Create Release step. Linux/Windows targets need their own
runners or a cross-compilation setup (e.g. `cross`), since the matrix currently
relies on a macOS runner.

> **Version caveat**: the version bump is ephemeral — it sets the version in the
> workflow workspace before building but is **not committed**, so in-repo
> `Cargo.toml` stays at its base version while git tags advance (`v1.0.x`).
> A from-source build (like homebrew-core's) therefore reports the `Cargo.toml`
> version, not the tag. Align `Cargo.toml`'s `version` with the release tag before
> relying on `lastrun --version` matching the published version.

## Homebrew (homebrew-core)

`lastrun` is published to the official **homebrew-core** tap.

- `.github/workflows/homebrew.yml` runs `dawidd6/action-homebrew-bump-formula`
  on each published release (dispatched from `build.yml` with a PAT, because
  releases made by `GITHUB_TOKEN` don't fire the `release` event). It opens a
  version-bump PR against homebrew-core.
- **Required secret**: `HOMEBREW_GITHUB_TOKEN` — a classic PAT (`public_repo` +
  `workflow`) on an account that has forked `Homebrew/homebrew-core`. This must
  be added in the repo settings; it cannot be created from code.
- **First submission is manual**: homebrew-core reviews the initial formula. Use
  `packaging/homebrew/lastrun.rb` as the starting point (build-from-source,
  `depends_on "rust" => :build`). Fill in the release tag URL and the source
  tarball `sha256`, then open the PR by hand. The workflow automates every
  bump after acceptance.

## Conventions

- Version lives in git tags (`v*`), bumped automatically (patch) on release.
- Dependabot PRs are auto-merged (`.github/workflows/auto-merge.yml`); PRs run
  `.github/workflows/test.yml`.
- Always run `task test` before committing.
