# LastRun

A utility to track when tasks were last run.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

## Overview

LastRun is a command-line utility that helps you track task execution history. It's perfect for monitoring cron jobs, scheduled tasks, or any recurring operations where you need to know when something last ran and whether it succeeded.

## Features

- ✅ Track task start, completion, and failure times
- 📊 View task history and logs
- 🔍 Filter and search task executions
- 🗑️ Clean up old task records
- 🔄 Reset database when needed
- 🎨 Pretty table output or JSON format
- 🤫 Quiet mode for scripting
- ⚡ Shell completion support

## Installation

### From Source

```bash
git clone https://github.com/yourusername/LastRun.git
cd LastRun
cargo build --release
cp target/release/lastrun /usr/local/bin/
```

## Quick Start

```bash
# Start tracking a task
lastrun start --id my-task

# Complete the task
lastrun complete --id my-task

# List all tasks
lastrun list

# View task logs
lastrun logs --id my-task
```

## Usage

### Start a Task

Begin tracking a new task execution:

```bash
lastrun start --id my-task
```

Or using short options:
```bash
lastrun start -i my-task
```

### Complete a Task

Mark a task as successfully completed:

```bash
lastrun complete --id my-task
```

### Mark Task as Failed

Record a task failure:

```bash
lastrun fail --id my-task
```

### List All Tasks

Display all tracked tasks:

```bash
lastrun list
```

### View Task Logs

Display logs for tasks:

```bash
lastrun logs
```

To filter logs by a specific task ID:

```bash
lastrun logs --id my-task
```

Or using short options:
```bash
lastrun logs -i my-task
```

To change the number of logs displayed (default is 20):

```bash
lastrun logs --limit 50
```

Or combine both options:
```bash
lastrun logs -i my-task -l 50
```

### Reset Database

Reset the tasks database, rebuilding the tables:

```bash
lastrun reset
```

### Delete Task Records

Delete a task and all its log entries:

```bash
lastrun delete --id my-task
```

Or using the short option:
```bash
lastrun delete -i my-task
```

### Quiet Mode

Add the `-q` or `--quiet` flag to suppress output messages:

```bash
lastrun start --id my-task -q
```

### Command Line Auto Completion

To enable zsh auto-completion, add the following to your `~/.zshrc`:

```sh
source <(lastrun completion zsh)
```

Restart your terminal or run `source ~/.zshrc` to activate tab completion for lastrun commands and options.

For bash completion:
```bash
source <(lastrun completion bash)
```

## Examples

See the [examples/](examples/) directory for more usage examples:
- [basic_usage.sh](examples/basic_usage.sh) - Simple task tracking
- [cron_integration.sh](examples/cron_integration.sh) - Using with cron jobs

## Development

### Building

```bash
cargo build
```

Or using the task runner:
```bash
task build
```

### Running Tests

```bash
cargo test
```

Or:
```bash
task test
```

### Available Tasks

View all available tasks:
```bash
task --list
```

## Project Structure

```
LastRun/
├── src/              # Source code
│   ├── main.rs       # Entry point
│   ├── lib.rs        # Library root
│   ├── cli.rs        # Command-line interface
│   ├── db.rs         # Database operations
│   ├── model.rs      # Data models
│   ├── error.rs      # Error handling
│   ├── format.rs     # Output formatting
│   └── display.rs    # Display logic
├── tests/            # Integration and unit tests
├── docs/             # Documentation
├── examples/         # Usage examples
└── Cargo.toml        # Project metadata
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for detailed architecture documentation.

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Author

Eric Veenendaal
