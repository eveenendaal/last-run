# Contributing to LastRun

Thank you for your interest in contributing to LastRun!

## Development Setup

1. Install Go (see [`go.mod`](go.mod) for the required version) from
   <https://go.dev/dl/>.

2. Clone the repository:
   ```bash
   git clone https://github.com/eveenendaal/last-run.git
   cd last-run
   ```

3. Build the project:
   ```bash
   go build ./...
   ```

## Running Tests

Run the test suite:
```bash
go test ./...
```

Or using the Taskfile:
```bash
task test
```

`task test` is the canonical pre-commit check — please run it before
pushing. CI (`.github/workflows/test.yml`) runs the same command on every
pull request.

## Building

Build a release binary:
```bash
task build
```

This produces `dist/lastrun` plus a matching `.sha256`. Cross-compile for
another platform with `GOOS`/`GOARCH`, e.g.:
```bash
GOOS=windows GOARCH=amd64 task build
```

## Code Style

This project follows standard Go conventions. Run `gofmt -w .` to format your
code and `go vet ./...` to check for common issues.

## Submitting Changes

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests to ensure everything works
5. Submit a pull request

## Questions?

Feel free to open an issue for any questions or concerns.
