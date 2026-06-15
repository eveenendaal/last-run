// Package display renders task data as plain-text output: ANSI-colored status
// messages, a JSON status snapshot, and a log table.
package display

// ANSI color constants used by the CLI's printf-style status messages.
const (
	BOLD  = "\x1b[1m"
	RESET = "\x1b[0m"
	GREEN = "\x1b[32m"
	RED   = "\x1b[31m"
	WHITE = "\x1b[97m"
)
