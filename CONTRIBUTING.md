# Contributing to LastRun

Thank you for your interest in contributing to LastRun!

## Development Setup

1. Install Rust (if not already installed):
   ```bash
   curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
   ```

2. Clone the repository:
   ```bash
   git clone https://github.com/eveenendaal/last-run.git
   cd last-run
   ```

3. Build the project:
   ```bash
   cargo build
   ```

## Running Tests

Run the test suite:
```bash
cargo test
```

Or using the Taskfile:
```bash
task test
```

`task test` is the canonical pre-commit check — please run it before
pushing. CI (`.github/workflows/test.yml`) runs the same command on every
pull request.

## Building

Build the release version:
```bash
cargo build --release
```

Or using the Taskfile:
```bash
task build
```

## Code Style

This project follows standard Rust conventions. Run `cargo fmt` to format your code and `cargo clippy` to check for common issues.

## Submitting Changes

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests to ensure everything works
5. Submit a pull request

## Questions?

Feel free to open an issue for any questions or concerns.
