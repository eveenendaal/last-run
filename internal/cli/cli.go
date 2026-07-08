// Package cli defines the cobra command tree, the shared ShouldRunTask logic,
// and the command handlers that mirror the original Rust main.rs dispatch.
package cli

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/fang"
	"github.com/eveenendaal/last-run/internal/apperr"
	"github.com/eveenendaal/last-run/internal/db"
	"github.com/eveenendaal/last-run/internal/display"
	"github.com/eveenendaal/last-run/internal/format"
	"github.com/eveenendaal/last-run/internal/model"
	"github.com/eveenendaal/last-run/internal/settings"
	"github.com/eveenendaal/last-run/internal/tui"
	"github.com/spf13/cobra"
)

// Version is the release version, injected from main via ldflags.
var Version = "dev"

// Execute wires the cobra command tree to the CLI entrypoint.
func Execute(version string) error {
	Version = version
	root := NewRootCmd()
	return fang.Execute(context.Background(), root, fang.WithVersion(version))
}

// appContext carries the shared database handle and global flags through the
// command handlers.
type appContext struct {
	db     *sql.DB
	dbPath string
	quiet  bool
}

// ShouldRunTask reports whether a task is due and a human-readable explanation,
// shared by the `check` command and the status TUI's colouring.
func ShouldRunTask(lastRun time.Time, duration time.Duration) (bool, string) {
	timeSince := time.Now().UTC().Sub(lastRun)

	if timeSince >= duration {
		return true, fmt.Sprintf(
			"Task is due (last run: %s, %s ago)",
			format.FormatRFC3339(lastRun), format.FormatDuration(timeSince),
		)
	}
	return false, fmt.Sprintf(
		"Task is not due yet (last run: %s, %s ago, threshold: %s)",
		format.FormatRFC3339(lastRun), format.FormatDuration(timeSince), format.FormatDuration(duration),
	)
}

// NewRootCmd builds the full command tree.
func NewRootCmd() *cobra.Command {
	ctx := &appContext{}

	var dbPath string
	var quiet bool

	root := &cobra.Command{
		Use:           "lastrun",
		Short:         "A utility to track when tasks were last run",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			resolvedPath, err := db.ResolveDBPath(dbPath)
			if err != nil {
				return err
			}
			if parent := filepath.Dir(resolvedPath); parent != "" {
				if err := os.MkdirAll(parent, 0o755); err != nil {
					return err
				}
			}
			conn, err := db.Open(resolvedPath)
			if err != nil {
				return err
			}
			if err := db.InitDB(conn); err != nil {
				return err
			}
			ctx.db = conn
			ctx.dbPath = resolvedPath
			ctx.quiet = quiet
			return nil
		},
		PersistentPostRunE: func(_ *cobra.Command, _ []string) error {
			if ctx.db != nil {
				return ctx.db.Close()
			}
			return nil
		},
	}

	root.PersistentFlags().StringVar(&dbPath, "db-path", os.Getenv("LASTRUN_DB_PATH"), "Path to the database file")
	root.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppress output messages")

	root.AddGroup(
		&cobra.Group{ID: "workflow", Title: "Daily workflow:"},
		&cobra.Group{ID: "logs", Title: "Logs & history:"},
		&cobra.Group{ID: "tasks", Title: "Task management:"},
		&cobra.Group{ID: "config", Title: "Configuration & tooling:"},
	)

	root.AddCommand(
		newStartCmd(ctx),
		newUpdateCmd(ctx),
		newDoneCmd(ctx),
		newCheckCmd(ctx),
		newStatusCmd(ctx),
		newLogsCmd(ctx),
		newArchiveCmd(ctx),
		newSetRetentionCmd(ctx),
		newClearCmd(ctx),
		newDeleteCmd(ctx),
		newResetCmd(ctx),
		newSettingsCmd(ctx),
	)

	return root
}

func newStartCmd(ctx *appContext) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:     "start",
		Short:   "Start a task",
		GroupID: "workflow",
		RunE: func(_ *cobra.Command, _ []string) error {
			if id == "" {
				return apperr.ErrMissingTaskID
			}
			task, err := model.Ensure(ctx.db, id, ctx.quiet)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			task.StartTime = &now
			task.LastRun = nil
			if err := task.Update(ctx.db); err != nil {
				return err
			}
			if !ctx.quiet {
				fmt.Printf("%s%sTask %s%s%s started at %s%s%s\n",
					display.BOLD, display.GREEN, display.WHITE, task.ID, display.GREEN,
					display.WHITE, format.FormatDatetime(*task.StartTime), display.RESET)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&id, "id", "i", "", "Task ID to start")
	return cmd
}

func runDone(ctx *appContext, id string) error {
	if id == "" {
		return apperr.ErrMissingTaskID
	}
	task, err := model.Ensure(ctx.db, id, ctx.quiet)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	task.LastRun = &now
	if err := task.Update(ctx.db); err != nil {
		return err
	}

	elapsedMsg := ""
	if task.StartTime != nil {
		elapsed := format.FormatDuration(now.Sub(*task.StartTime))
		elapsedMsg = fmt.Sprintf("%s. Elapsed time: %s%s%s", display.GREEN, display.WHITE, elapsed, display.GREEN)
	}

	if !ctx.quiet {
		fmt.Printf("%s%sTask %s%s%s finished at %s%s%s%s\n",
			display.BOLD, display.GREEN, display.WHITE, task.ID, display.GREEN,
			display.WHITE, format.FormatDatetime(*task.LastRun), elapsedMsg, display.RESET)
	}

	return autoArchive(ctx)
}

func newUpdateCmd(ctx *appContext) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Update a task's last run time",
		GroupID: "workflow",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDone(ctx, id)
		},
	}
	cmd.Flags().StringVarP(&id, "id", "i", "", "Task ID to update")
	return cmd
}

func newDoneCmd(ctx *appContext) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:     "done",
		Short:   "Synonym for update",
		GroupID: "workflow",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDone(ctx, id)
		},
	}
	cmd.Flags().StringVarP(&id, "id", "i", "", "Task ID to mark as done")
	return cmd
}

func newCheckCmd(ctx *appContext) *cobra.Command {
	var id, duration string
	cmd := &cobra.Command{
		Use:     "check",
		Short:   "Check if a task is due to run",
		GroupID: "workflow",
		RunE: func(_ *cobra.Command, _ []string) error {
			if id == "" {
				return apperr.ErrMissingTaskID
			}
			dur, err := format.ParseDuration(duration)
			if err != nil {
				return apperr.NewDurationParseError(err.Error())
			}

			task, err := model.Select(ctx.db, id)
			if err != nil {
				return err
			}
			if task == nil {
				if !ctx.quiet {
					fmt.Printf("%s%sTask %s%s%s does not exist yet. It is considered due.%s\n",
						display.BOLD, display.RED, display.WHITE, id, display.RED, display.RESET)
				}
				os.Exit(1)
			}

			shouldExitDue := false
			if task.LastRun != nil {
				shouldRun, message := ShouldRunTask(*task.LastRun, dur)
				if !ctx.quiet {
					color := display.GREEN
					if shouldRun {
						color = display.RED
					}
					fmt.Printf("%s%s%s%s\n", display.BOLD, color, message, display.RESET)
				}
				if err := db.UpdateTaskDuration(ctx.db, task.ID, int64(dur/time.Second)); err != nil {
					return err
				}
				shouldExitDue = shouldRun
			} else {
				if !ctx.quiet {
					fmt.Printf("%s%sTask %s%s%s has no recorded last run. It is considered due.%s\n",
						display.BOLD, display.RED, display.WHITE, task.ID, display.RED, display.RESET)
				}
				shouldExitDue = true
			}

			if shouldExitDue {
				closeDB(ctx)
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&id, "id", "i", "", "Task ID to check")
	cmd.Flags().StringVarP(&duration, "duration", "d", "24h", "Duration threshold (e.g., 24h, 7d)")
	return cmd
}

func newStatusCmd(ctx *appContext) *cobra.Command {
	var id, sort string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Display current status of all tasks",
		GroupID: "workflow",
		RunE: func(_ *cobra.Command, _ []string) error {
			idPtr := optStr(id)
			if jsonOut {
				tasks, err := db.GetAllTasks(ctx.db, idPtr)
				if err != nil {
					return err
				}
				if !ctx.quiet {
					display.PrintTaskStatusJSON(tasks)
				}
				return nil
			}
			sortCol, err := parseSortColumn(sort)
			if err != nil {
				return err
			}
			return tui.RunTUI(ctx.db, idPtr, sortCol)
		},
	}
	cmd.Flags().StringVarP(&id, "id", "i", "", "Filter tasks by ID")
	cmd.Flags().StringVarP(&sort, "sort", "s", "last-run", "Column to sort by (task, status, duration, elapsed, last-run)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output status in JSON format")
	return cmd
}

func newLogsCmd(ctx *appContext) *cobra.Command {
	var id string
	var limit int
	cmd := &cobra.Command{
		Use:     "logs",
		Short:   "Display execution logs for tasks",
		GroupID: "logs",
		RunE: func(_ *cobra.Command, _ []string) error {
			logs, err := db.GetTaskLogs(ctx.db, optStr(id), limit)
			if err != nil {
				return err
			}
			if !ctx.quiet {
				display.PrintTaskLogs(logs)
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "l", 20, "Limit number of logs to show (0 for all)")
	cmd.Flags().StringVarP(&id, "id", "i", "", "Filter logs by task ID")
	return cmd
}

func newArchiveCmd(ctx *appContext) *cobra.Command {
	var olderThan, id string
	var yes bool
	cmd := &cobra.Command{
		Use:     "archive",
		Short:   "Delete log entries older than a specified period (defaults to the stored retention setting, or 30d)",
		GroupID: "logs",
		RunE: func(_ *cobra.Command, _ []string) error {
			var olderThanStr string
			var duration time.Duration
			if olderThan != "" {
				d, err := format.ParseDuration(olderThan)
				if err != nil {
					return apperr.NewDurationParseError(err.Error())
				}
				olderThanStr = olderThan
				duration = d
			} else {
				seconds, ok, err := db.GetLogRetentionSeconds(ctx.db)
				if err != nil {
					return err
				}
				if !ok {
					seconds = 30 * 24 * 3600
				}
				olderThanStr = fmt.Sprintf("%dd", seconds/(24*3600))
				duration = time.Duration(seconds) * time.Second
			}

			cutoff := time.Now().UTC().Add(-duration)
			idPtr := optStr(id)

			count, err := db.CountOldLogs(ctx.db, cutoff, idPtr)
			if err != nil {
				return err
			}

			if count == 0 {
				if !ctx.quiet {
					fmt.Printf("%s%sNo log entries found older than %s.%s\n",
						display.BOLD, display.GREEN, olderThanStr, display.RESET)
				}
				return nil
			}

			if !ctx.quiet {
				scope := " across all tasks"
				if id != "" {
					scope = fmt.Sprintf(" for task %s%s%s", display.WHITE, id, display.GREEN)
				}
				fmt.Printf("%s%sArchive logs%s%s\n", display.BOLD, display.GREEN, scope, display.RESET)
				fmt.Printf("  Keeping entries from: %s%s%s\n", display.WHITE, cutoff.UTC().Format("2006-01-02"), display.RESET)
				fmt.Printf("  Entries to delete:    %s%s%d%s\n", display.WHITE, display.BOLD, count, display.RESET)
				fmt.Println()
			}

			confirmed := yes
			if !confirmed {
				fmt.Printf("Permanently delete %d log %s? [y/N]: ", count, plural(count))
				reader := bufio.NewReader(os.Stdin)
				input, _ := reader.ReadString('\n')
				input = strings.ToLower(strings.TrimSpace(input))
				confirmed = input == "y" || input == "yes"
			}

			if confirmed {
				deleted, err := db.DeleteOldLogs(ctx.db, cutoff, idPtr)
				if err != nil {
					return err
				}
				if !ctx.quiet {
					fmt.Printf("%s%sDeleted %d log %s.%s\n",
						display.BOLD, display.GREEN, deleted, plural(deleted), display.RESET)
				}
			} else if !ctx.quiet {
				fmt.Printf("%sCancelled.%s\n", display.RED, display.RESET)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&olderThan, "older-than", "o", "", "How far back to keep logs (e.g. 30d, 2w, 3m, 24h). Entries older than this are deleted.")
	cmd.Flags().StringVarP(&id, "id", "i", "", "Limit archiving to a specific task ID (default: all tasks)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt and delete immediately")
	return cmd
}

func newSetRetentionCmd(ctx *appContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "set-retention <duration>",
		Short:   "Set the log retention period for automatic cleanup (e.g. 30d, 2w, 3m, 24h). Pass \"off\" to disable.",
		GroupID: "logs",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			normalized := strings.TrimSpace(args[0])
			disabled := strings.EqualFold(normalized, "off") || normalized == "0"
			if err := db.SetLogRetention(ctx.db, normalized); err != nil {
				return err
			}
			if !ctx.quiet {
				if disabled {
					fmt.Printf("%s%sLog retention disabled — auto-cleanup turned off.%s\n",
						display.BOLD, display.GREEN, display.RESET)
				} else {
					fmt.Printf("%s%sLog retention set to %s%s%s. Old logs will be auto-cleaned on each `done`/`update`.%s\n",
						display.BOLD, display.GREEN, display.WHITE, normalized, display.GREEN, display.RESET)
				}
			}
			return nil
		},
	}
	return cmd
}

func newClearCmd(ctx *appContext) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:     "clear",
		Short:   "Clear a task's start and done values",
		GroupID: "tasks",
		RunE: func(_ *cobra.Command, _ []string) error {
			if id == "" {
				return apperr.ErrMissingTaskID
			}
			task, err := model.Select(ctx.db, id)
			if err != nil {
				return err
			}
			if task == nil {
				if !ctx.quiet {
					fmt.Printf("%s%sTask %s%s%s does not exist.%s\n",
						display.BOLD, display.RED, display.WHITE, id, display.RED, display.RESET)
				}
				return nil
			}
			task.LastRun = nil
			task.StartTime = nil
			if err := task.Update(ctx.db); err != nil {
				return err
			}
			if !ctx.quiet {
				fmt.Printf("%s%sTask %s%s%s cleared (start and done values reset).%s\n",
					display.BOLD, display.GREEN, display.WHITE, id, display.GREEN, display.RESET)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&id, "id", "i", "", "Task ID to clear")
	return cmd
}

func newDeleteCmd(ctx *appContext) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:     "delete",
		Short:   "Delete a task and its log records by ID",
		GroupID: "tasks",
		RunE: func(_ *cobra.Command, _ []string) error {
			if id == "" {
				return apperr.ErrMissingTaskID
			}
			logsDeleted, err := db.DeleteTaskLogs(ctx.db, id)
			if err != nil {
				return err
			}
			taskDeleted, err := db.DeleteTask(ctx.db, id)
			if err != nil {
				return err
			}
			if !ctx.quiet {
				if taskDeleted > 0 {
					fmt.Printf("%s%sTask %s%s%s deleted. %d log entries removed.%s\n",
						display.BOLD, display.GREEN, display.WHITE, id, display.GREEN, logsDeleted, display.RESET)
				} else {
					fmt.Printf("%s%sNo task found with ID: %s%s%s. %d log entries removed.%s\n",
						display.BOLD, display.RED, display.WHITE, id, display.RED, logsDeleted, display.RESET)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&id, "id", "i", "", "Task ID to delete")
	return cmd
}

func newResetCmd(ctx *appContext) *cobra.Command {
	return &cobra.Command{
		Use:     "reset",
		Short:   "Reset the tasks database",
		GroupID: "tasks",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := db.CleanDB(ctx.db); err != nil {
				return err
			}
			if !ctx.quiet {
				fmt.Printf("%s%sTasks table has been rebuilt.%s\n", display.BOLD, display.GREEN, display.RESET)
			}
			return nil
		},
	}
}

func newSettingsCmd(ctx *appContext) *cobra.Command {
	return &cobra.Command{
		Use:     "settings",
		Short:   "Interactively view and edit settings (e.g. log retention, DB location)",
		GroupID: "config",
		RunE: func(_ *cobra.Command, _ []string) error {
			return settings.RunSettingsTUI(ctx.db, ctx.dbPath)
		},
	}
}

func autoArchive(ctx *appContext) error {
	seconds, ok, err := db.GetLogRetentionSeconds(ctx.db)
	if err != nil {
		return err
	}
	retention := int64(30 * 24 * 3600)
	if ok {
		retention = seconds
	}
	if retention <= 0 {
		return nil
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retention) * time.Second)
	deleted, err := db.DeleteOldLogs(ctx.db, cutoff, nil)
	if err != nil {
		return err
	}
	if !ctx.quiet && deleted > 0 {
		fmt.Printf("%sAuto-cleaned %d old log %s.%s\n", display.GREEN, deleted, plural(deleted), display.RESET)
	}
	return nil
}

func parseSortColumn(s string) (tui.SortCol, error) {
	switch s {
	case "task":
		return tui.SortTask, nil
	case "status":
		return tui.SortStatus, nil
	case "duration":
		return tui.SortDuration, nil
	case "elapsed":
		return tui.SortElapsed, nil
	case "last-run":
		return tui.SortLastRun, nil
	default:
		return tui.SortLastRun, fmt.Errorf("invalid sort column %q (expected task, status, duration, elapsed, last-run)", s)
	}
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func plural(n int64) string {
	if n == 1 {
		return "entry"
	}
	return "entries"
}

func closeDB(ctx *appContext) {
	if ctx.db != nil {
		_ = ctx.db.Close()
	}
}
