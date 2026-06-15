// Command lastrun tracks when tasks were last run — start/complete times,
// history, duration thresholds, and an interactive status TUI. State is kept in
// a single SQLite database.
package main

import (
	"context"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/eveenendaal/last-run/internal/cli"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cli.Version = version
	root := cli.NewRootCmd()
	if err := fang.Execute(context.Background(), root, fang.WithVersion(version)); err != nil {
		os.Exit(1)
	}
}
