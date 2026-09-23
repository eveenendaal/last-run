// Package cli defines the cobra command tree, the ShouldRunTask logic behind
// `check`, and the command handlers.
package cli

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
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

// ErrTaskDue is returned by `check` when the task is due. It is not printed;
// it only makes the process exit non-zero.
var ErrTaskDue = errors.New("task is due")

// Execute runs the CLI with the given release version.
func Execute(version string) error {
	app := &appContext{}
	defer app.close()
	return fang.Execute(context.Background(), newRootCmd(app, version),
		fang.WithVersion(version),
		fang.WithErrorHandler(func(w io.Writer, styles fang.Styles, err error) {
			if !errors.Is(err, ErrTaskDue) {
				fang.DefaultErrorHandler(w, styles, err)
			}
		}),
	)
}

// appContext carries the shared database handle and global flags through the
// command handlers.
type appContext struct {
	db     *sql.DB
	dbPath string
	quiet  bool
	out    io.Writer
}

func (a *appContext) close() {
	if a.db != nil {
		_ = a.db.Close()
		a.db = nil
	}
}

// printf writes a formatted message unless --quiet was given.
func (a *appContext) printf(format string, args ...any) {
	if !a.quiet {
		fmt.Fprintf(a.out, format, args...)
	}
}

// ShouldRunTask reports whether a task last run at lastRun is due under the
// given threshold, along with a human-readable explanation.
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

// newRootCmd builds the full command tree around app.
func newRootCmd(app *appContext, version string) *cobra.Command {
	var dbPath string

	root := &cobra.Command{
		Use:           "lastrun",
		Short:         "A utility to track when tasks were last run",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			resolvedPath, err := db.ResolveDBPath(dbPath)
			if err != nil {
				return err
			}
			conn, err := db.Open(resolvedPath)
			if err != nil {
				return err
			}
			app.db = conn
			app.dbPath = resolvedPath
			app.out = cmd.OutOrStdout()
			return db.InitDB(conn)
		},
		PersistentPostRun: func(_ *cobra.Command, _ []string) {
			app.close()
		},
	}

	root.PersistentFlags().StringVar(&dbPath, "db-path", os.Getenv("LASTRUN_DB_PATH"), "Path to the database file")
	root.PersistentFlags().BoolVarP(&app.quiet, "quiet", "q", false, "Suppress output messages")

	root.AddGroup(
		&cobra.Group{ID: "workflow", Title: "Daily workflow:"},
		&cobra.Group{ID: "logs", Title: "Logs & history:"},
		&cobra.Group{ID: "tasks", Title: "Task management:"},
		&cobra.Group{ID: "config", Title: "Configuration & tooling:"},
	)

	root.AddCommand(
		newStartCmd(app),
		newDoneCmd(app),
		newCheckCmd(app),
		newStatusCmd(app),
		newLogsCmd(app),
		newArchiveCmd(app),
		newSetRetentionCmd(app),
		newClearCmd(app),
		newDeleteCmd(app),
		newResetCmd(app),
		newSettingsCmd(app),
	)

	return root
}

// idFlag registers the common --id/-i flag on cmd.
func idFlag(cmd *cobra.Command, id *string, usage string) {
	cmd.Flags().StringVarP(id, "id", "i", "", usage)
}

// ensureTask loads or creates the task, announcing a newly created one.
func ensureTask(app *appContext, id string) (*model.Task, error) {
	if id == "" {
		return nil, apperr.ErrMissingTaskID
	}
	task, created, err := model.Ensure(app.db, id)
	if created {
		app.printf("No record found for task ID: %s\n", id)
	}
	return task, err
}

func newStartCmd(app *appContext) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:     "start",
		Short:   "Start a task",
		GroupID: "workflow",
		RunE: func(_ *cobra.Command, _ []string) error {
			task, err := ensureTask(app, id)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			task.StartTime = &now
			task.LastRun = nil
			if err := task.Update(app.db); err != nil {
				return err
			}
			app.printf("%s%sTask %s%s%s started at %s%s%s\n",
				display.BOLD, display.GREEN, display.WHITE, task.ID, display.GREEN,
				display.WHITE, format.FormatDatetime(now), display.RESET)
			return nil
		},
	}
	idFlag(cmd, &id, "Task ID to start")
	return cmd
}

func newDoneCmd(app *appContext) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:     "done",
		Aliases: []string{"update"},
		Short:   "Mark a task as done, recording its last run time (alias: update)",
		GroupID: "workflow",
		RunE: func(_ *cobra.Command, _ []string) error {
			task, err := ensureTask(app, id)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			task.LastRun = &now
			if err := task.Update(app.db); err != nil {
				return err
			}

			elapsedMsg := ""
			if task.StartTime != nil {
				elapsedMsg = fmt.Sprintf("%s. Elapsed time: %s%s%s", display.GREEN, display.WHITE,
					format.FormatDuration(now.Sub(*task.StartTime)), display.GREEN)
			}
			app.printf("%s%sTask %s%s%s finished at %s%s%s%s\n",
				display.BOLD, display.GREEN, display.WHITE, task.ID, display.GREEN,
				display.WHITE, format.FormatDatetime(now), elapsedMsg, display.RESET)

			return autoArchive(app)
		},
	}
	idFlag(cmd, &id, "Task ID to mark as done")
	return cmd
}

func newCheckCmd(app *appContext) *cobra.Command {
	var id, duration string
	cmd := &cobra.Command{
		Use:     "check",
		Short:   "Check if a task is due to run (exits 1 when due)",
		GroupID: "workflow",
		RunE: func(_ *cobra.Command, _ []string) error {
			if id == "" {
				return apperr.ErrMissingTaskID
			}
			dur, err := format.ParseDuration(duration)
			if err != nil {
				return apperr.NewDurationParseError(err.Error())
			}

			task, err := model.Select(app.db, id)
			if err != nil {
				return err
			}
			switch {
			case task == nil:
				app.printf("%s%sTask %s%s%s does not exist yet. It is considered due.%s\n",
					display.BOLD, display.RED, display.WHITE, id, display.RED, display.RESET)
				return ErrTaskDue
			case task.LastRun == nil:
				app.printf("%s%sTask %s%s%s has no recorded last run. It is considered due.%s\n",
					display.BOLD, display.RED, display.WHITE, task.ID, display.RED, display.RESET)
				return ErrTaskDue
			}

			shouldRun, message := ShouldRunTask(*task.LastRun, dur)
			color := display.GREEN
			if shouldRun {
				color = display.RED
			}
			app.printf("%s%s%s%s\n", display.BOLD, color, message, display.RESET)
			if err := db.UpdateTaskDuration(app.db, task.ID, int64(dur/time.Second)); err != nil {
				return err
			}
			if shouldRun {
				return ErrTaskDue
			}
			return nil
		},
	}
	idFlag(cmd, &id, "Task ID to check")
	cmd.Flags().StringVarP(&duration, "duration", "d", "24h", "Duration threshold (e.g., 24h, 7d)")
	return cmd
}

func newStatusCmd(app *appContext) *cobra.Command {
	var id, sort string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Display current status of all tasks",
		GroupID: "workflow",
		RunE: func(_ *cobra.Command, _ []string) error {
			if jsonOut {
				tasks, err := db.GetAllTasks(app.db, optStr(id))
				if err != nil || app.quiet {
					return err
				}
				return display.WriteTaskStatusJSON(app.out, tasks, time.Now().UTC())
			}
			sortCol, err := parseSortColumn(sort)
			if err != nil {
				return err
			}
			return tui.RunTUI(app.db, optStr(id), sortCol)
		},
	}
	idFlag(cmd, &id, "Filter tasks by ID")
	cmd.Flags().StringVarP(&sort, "sort", "s", "last-run", "Column to sort by (task, status, duration, elapsed, last-run)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output status in JSON format")
	return cmd
}

func newLogsCmd(app *appContext) *cobra.Command {
	var id string
	var limit int
	cmd := &cobra.Command{
		Use:     "logs",
		Short:   "Display execution logs for tasks",
		GroupID: "logs",
		RunE: func(_ *cobra.Command, _ []string) error {
			logs, err := db.GetTaskLogs(app.db, optStr(id), limit)
			if err != nil {
				return err
			}
			if !app.quiet {
				display.WriteTaskLogs(app.out, logs)
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "l", 20, "Limit number of logs to show (0 for all)")
	idFlag(cmd, &id, "Filter logs by task ID")
	return cmd
}

func newArchiveCmd(app *appContext) *cobra.Command {
	var olderThan, id string
	var yes bool
	cmd := &cobra.Command{
		Use:     "archive",
		Short:   "Delete log entries older than a specified period (defaults to the stored retention setting, or 30d)",
		GroupID: "logs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var duration time.Duration
			if olderThan != "" {
				d, err := format.ParseDuration(olderThan)
				if err != nil {
					return apperr.NewDurationParseError(err.Error())
				}
				duration = d
			} else {
				d, err := db.LogRetention(app.db)
				if err != nil {
					return err
				}
				if d == 0 { // auto-cleanup is off; a manual archive still uses the default
					d = db.DefaultLogRetention
				}
				duration = d
				olderThan = fmt.Sprintf("%dd", int64(d/(24*time.Hour)))
			}

			cutoff := time.Now().UTC().Add(-duration)
			idPtr := optStr(id)

			count, err := db.CountOldLogs(app.db, cutoff, idPtr)
			if err != nil {
				return err
			}
			if count == 0 {
				app.printf("%s%sNo log entries found older than %s.%s\n",
					display.BOLD, display.GREEN, olderThan, display.RESET)
				return nil
			}

			scope := " across all tasks"
			if id != "" {
				scope = fmt.Sprintf(" for task %s%s%s", display.WHITE, id, display.GREEN)
			}
			app.printf("%s%sArchive logs%s%s\n", display.BOLD, display.GREEN, scope, display.RESET)
			app.printf("  Keeping entries from: %s%s%s\n", display.WHITE, cutoff.Format("2006-01-02"), display.RESET)
			app.printf("  Entries to delete:    %s%s%d%s\n\n", display.WHITE, display.BOLD, count, display.RESET)

			if !yes {
				fmt.Fprintf(app.out, "Permanently delete %d log %s? [y/N]: ", count, plural(count))
				input, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				input = strings.ToLower(strings.TrimSpace(input))
				if input != "y" && input != "yes" {
					app.printf("%sCancelled.%s\n", display.RED, display.RESET)
					return nil
				}
			}

			deleted, err := db.DeleteOldLogs(app.db, cutoff, idPtr)
			if err != nil {
				return err
			}
			app.printf("%s%sDeleted %d log %s.%s\n",
				display.BOLD, display.GREEN, deleted, plural(deleted), display.RESET)
			return nil
		},
	}
	cmd.Flags().StringVarP(&olderThan, "older-than", "o", "", "How far back to keep logs (e.g. 30d, 2w, 3m, 24h). Entries older than this are deleted.")
	idFlag(cmd, &id, "Limit archiving to a specific task ID (default: all tasks)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt and delete immediately")
	return cmd
}

func newSetRetentionCmd(app *appContext) *cobra.Command {
	return &cobra.Command{
		Use:     "set-retention <duration>",
		Short:   "Set the log retention period for automatic cleanup (e.g. 30d, 2w, 3m, 24h). Pass \"off\" to disable.",
		GroupID: "logs",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			value := strings.TrimSpace(args[0])
			if err := db.SetLogRetention(app.db, value); err != nil {
				return err
			}
			if db.IsRetentionOff(value) {
				app.printf("%s%sLog retention disabled — auto-cleanup turned off.%s\n",
					display.BOLD, display.GREEN, display.RESET)
			} else {
				app.printf("%s%sLog retention set to %s%s%s. Old logs will be auto-cleaned on each `done`/`update`.%s\n",
					display.BOLD, display.GREEN, display.WHITE, value, display.GREEN, display.RESET)
			}
			return nil
		},
	}
}

func newClearCmd(app *appContext) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:     "clear",
		Short:   "Clear a task's start and done values",
		GroupID: "tasks",
		RunE: func(_ *cobra.Command, _ []string) error {
			if id == "" {
				return apperr.ErrMissingTaskID
			}
			task, err := model.Select(app.db, id)
			if err != nil {
				return err
			}
			if task == nil {
				app.printf("%s%sTask %s%s%s does not exist.%s\n",
					display.BOLD, display.RED, display.WHITE, id, display.RED, display.RESET)
				return nil
			}
			task.LastRun = nil
			task.StartTime = nil
			if err := task.Update(app.db); err != nil {
				return err
			}
			app.printf("%s%sTask %s%s%s cleared (start and done values reset).%s\n",
				display.BOLD, display.GREEN, display.WHITE, id, display.GREEN, display.RESET)
			return nil
		},
	}
	idFlag(cmd, &id, "Task ID to clear")
	return cmd
}

func newDeleteCmd(app *appContext) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:     "delete",
		Short:   "Delete a task and its log records by ID",
		GroupID: "tasks",
		RunE: func(_ *cobra.Command, _ []string) error {
			if id == "" {
				return apperr.ErrMissingTaskID
			}
			logsDeleted, err := db.DeleteTaskLogs(app.db, id)
			if err != nil {
				return err
			}
			taskDeleted, err := db.DeleteTask(app.db, id)
			if err != nil {
				return err
			}
			if taskDeleted > 0 {
				app.printf("%s%sTask %s%s%s deleted. %d log entries removed.%s\n",
					display.BOLD, display.GREEN, display.WHITE, id, display.GREEN, logsDeleted, display.RESET)
			} else {
				app.printf("%s%sNo task found with ID: %s%s%s. %d log entries removed.%s\n",
					display.BOLD, display.RED, display.WHITE, id, display.RED, logsDeleted, display.RESET)
			}
			return nil
		},
	}
	idFlag(cmd, &id, "Task ID to delete")
	return cmd
}

func newResetCmd(app *appContext) *cobra.Command {
	return &cobra.Command{
		Use:     "reset",
		Short:   "Reset the tasks database",
		GroupID: "tasks",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := db.CleanDB(app.db); err != nil {
				return err
			}
			app.printf("%s%sTasks table has been rebuilt.%s\n", display.BOLD, display.GREEN, display.RESET)
			return nil
		},
	}
}

func newSettingsCmd(app *appContext) *cobra.Command {
	return &cobra.Command{
		Use:     "settings",
		Short:   "Interactively view and edit settings (e.g. log retention, DB location)",
		GroupID: "config",
		RunE: func(_ *cobra.Command, _ []string) error {
			return settings.RunSettingsTUI(app.db, app.dbPath)
		},
	}
}

// autoArchive deletes log entries older than the configured retention period
// (DefaultLogRetention when unset). It does nothing when retention is off.
func autoArchive(app *appContext) error {
	retention, err := db.LogRetention(app.db)
	if err != nil || retention == 0 {
		return err
	}
	deleted, err := db.DeleteOldLogs(app.db, time.Now().UTC().Add(-retention), nil)
	if err != nil {
		return err
	}
	if deleted > 0 {
		app.printf("%sAuto-cleaned %d old log %s.%s\n", display.GREEN, deleted, plural(deleted), display.RESET)
	}
	return nil
}

var sortColumns = map[string]tui.SortCol{
	"task":     tui.SortTask,
	"status":   tui.SortStatus,
	"duration": tui.SortDuration,
	"elapsed":  tui.SortElapsed,
	"last-run": tui.SortLastRun,
}

func parseSortColumn(s string) (tui.SortCol, error) {
	if col, ok := sortColumns[s]; ok {
		return col, nil
	}
	return tui.SortLastRun, fmt.Errorf("invalid sort column %q (expected task, status, duration, elapsed, last-run)", s)
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
