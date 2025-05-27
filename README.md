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
source <(lastrun completions zsh)
```

Restart your terminal or run `source ~/.zshrc` to activate tab completion for lastrun commands and options.

