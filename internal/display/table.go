package display

import (
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/eveenendaal/last-run/internal/db"
	"github.com/eveenendaal/last-run/internal/format"
)

// WriteTaskLogs renders task logs as a bordered table with columns
// TASK ID | COMPLETION TIME | DURATION.
func WriteTaskLogs(w io.Writer, logs []db.LogRow) {
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true).Padding(0, 1)
	cellStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Padding(0, 1)

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("8"))).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return cellStyle
		}).
		Headers("TASK ID", "COMPLETION TIME", "DURATION")

	for _, l := range logs {
		t.Row(l.ID, format.FormatDatetime(l.EndTime), format.FormatDuration(time.Duration(l.ElapsedMs)*time.Millisecond))
	}
	if len(logs) == 0 {
		t.Row("No logs found", "", "")
	}

	fmt.Fprintln(w, t)
}
