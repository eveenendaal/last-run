package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/eveenendaal/last-run/internal/format"
	"github.com/eveenendaal/last-run/internal/tuiutil"
)

var (
	colDarkGray = lipgloss.Color("8")
	colWhite    = lipgloss.Color("15")
	colYellow   = lipgloss.Color("3")
	colGreen    = lipgloss.Color("2")
	colCyan     = lipgloss.Color("6")
	colRed      = lipgloss.Color("1")
)

func (m *model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	now := time.Now().UTC()

	var shortcuts []tuiutil.Shortcut
	if m.state == stateHistory {
		shortcuts = basicShortcuts(true)
	} else {
		shortcuts = basicShortcuts(false)
	}

	ctrlH := tuiutil.ControlsHeight(m.width, shortcuts)
	mainH := m.height - ctrlH
	if mainH < 3 {
		mainH = 3
	}

	var mainView string
	if m.state == stateHistory {
		mainView = m.renderHistory(mainH, now)
	} else {
		mainView = m.renderTable(mainH, now)
	}
	controls := tuiutil.RenderControls(m.width, shortcuts)
	base := mainView + "\n" + controls

	if m.state == stateConfirmDelete {
		content := fmt.Sprintf("Delete \"%s\" and all its history?  [y] Yes   [n] No", m.confirmDeleteID)
		box := tuiutil.CenteredBox(" Confirm Delete Task ", colRed, content)
		base = tuiutil.PlaceOverlay(base, box, m.width, m.height)
	}
	if m.state == stateHistory && m.historyConfirmDelete != "" {
		base = tuiutil.PlaceOverlay(base, m.logDeletePopup(now), m.width, m.height)
	}
	if m.showHelp {
		box := tuiutil.RenderHelpModal(allShortcuts(m.state == stateHistory))
		base = tuiutil.PlaceOverlay(base, box, m.width, m.height)
	}
	return base
}

func (m *model) renderTable(mainH int, now time.Time) string {
	innerW := m.width - 2
	m.pageSize = mainH - 4
	if m.pageSize < 1 {
		m.pageSize = 1
	}

	statusW, durW, elapW, lastW := 10, 12, 12, 12
	if m.width < 60 {
		statusW, durW, elapW, lastW = 8, 7, 7, 7
	}
	taskW := innerW - 2 - (statusW + durW + elapW + lastW) - 4
	if taskW < 5 {
		taskW = 5
	}

	headerCell := func(label string, col SortCol, w int) string {
		text := label
		if col == m.sortCol {
			if m.sortAsc {
				text += " ▲"
			} else {
				text += " ▼"
			}
		}
		st := lipgloss.NewStyle().Foreground(colWhite).Bold(true)
		if col == m.sortCol {
			st = lipgloss.NewStyle().Foreground(colYellow).Bold(true)
		}
		return st.Render(tuiutil.Fit(text, w))
	}

	header := "  " + strings.Join([]string{
		headerCell("Task", SortTask, taskW),
		headerCell("Status", SortStatus, statusW),
		headerCell("Duration", SortDuration, durW),
		headerCell("Elapsed", SortElapsed, elapW),
		headerCell("Last Run", SortLastRun, lastW),
	}, " ")

	lines := []string{header, ""}

	if len(m.tasks) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(colDarkGray).Render("No tasks found"))
	} else {
		for i, t := range m.tasks {
			marker := "  "
			if i == m.cursor {
				marker = "▶ "
			}
			cells := []string{
				tuiutil.Fit(t.id, taskW),
				tuiutil.Fit(taskStatusStr(t, now), statusW),
				tuiutil.Fit(durationCell(t), durW),
				tuiutil.Fit(elapsedCell(t, now), elapW),
				tuiutil.Fit(lastRunCell(t, now), lastW),
			}
			rowPlain := tuiutil.Fit(marker+strings.Join(cells, " "), innerW)
			st := lipgloss.NewStyle().Foreground(taskColor(t, now))
			if i == m.cursor {
				st = st.Reverse(true)
			}
			lines = append(lines, st.Render(rowPlain))
		}
	}

	tsFmt := "15:04:05"
	if m.width >= 50 {
		tsFmt = "Jan 2, 15:04:05"
	}
	right := " " + m.lastUpdated.Local().Format(tsFmt) + " "

	return tuiutil.Panel(m.width, mainH, " Last Run Status ", right, colDarkGray, strings.Join(lines, "\n"))
}

func durationCell(t taskRow) string {
	if t.duration == nil {
		return "-"
	}
	return format.FormatDuration(time.Duration(*t.duration) * time.Second)
}

func elapsedCell(t taskRow, now time.Time) string {
	switch {
	case t.startTime != nil && t.lastRun != nil && t.startTime.Before(*t.lastRun):
		return format.FormatDuration(t.lastRun.Sub(*t.startTime))
	case t.startTime != nil && t.lastRun == nil:
		return format.FormatDuration(now.Sub(*t.startTime))
	default:
		return "-"
	}
}

func lastRunCell(t taskRow, now time.Time) string {
	if t.lastRun == nil {
		return "-"
	}
	return format.FormatDuration(now.Sub(*t.lastRun))
}

func (m *model) renderHistory(mainH int, now time.Time) string {
	innerW := m.width - 2
	m.historyPageSize = mainH - 5
	if m.historyPageSize < 1 {
		m.historyPageSize = 1
	}

	// Stats line.
	var statsLine string
	if avg, min, max, freq, hasFreq, ok := m.historyStats(); ok {
		dg := lipgloss.NewStyle().Foreground(colDarkGray)
		var b strings.Builder
		b.WriteString(dg.Render("  Avg: "))
		b.WriteString(lipgloss.NewStyle().Foreground(colWhite).Bold(true).Render(format.FormatDuration(msDur(avg))))
		b.WriteString(dg.Render("   Min: "))
		b.WriteString(lipgloss.NewStyle().Foreground(colGreen).Render(format.FormatDuration(msDur(min))))
		b.WriteString(dg.Render("   Max: "))
		b.WriteString(lipgloss.NewStyle().Foreground(colYellow).Render(format.FormatDuration(msDur(max))))
		if hasFreq {
			b.WriteString(dg.Render("   Freq: every ~"))
			b.WriteString(lipgloss.NewStyle().Foreground(colCyan).Render(format.FormatDuration(msDur(freq))))
		}
		statsLine = b.String()
	} else {
		statsLine = lipgloss.NewStyle().Foreground(colDarkGray).Render("  No run history recorded.")
	}

	idxW, atW, durW := 4, 21, 12
	agoW := innerW - 2 - (idxW + atW + durW) - 3
	if agoW < 5 {
		agoW = 5
	}

	hStyle := lipgloss.NewStyle().Foreground(colDarkGray).Bold(true)
	header := "  " + strings.Join([]string{
		hStyle.Render(tuiutil.Fit("#", idxW)),
		hStyle.Render(tuiutil.Fit("Completed At", atW)),
		hStyle.Render(tuiutil.Fit("Duration", durW)),
		hStyle.Render(tuiutil.Fit("Time Ago", agoW)),
	}, " ")

	lines := []string{statsLine, "", header}

	if len(m.historyLogs) == 0 {
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(colDarkGray).Render("No log entries found."))
	} else {
		for i, e := range m.historyLogs {
			ago := now.Sub(e.endTime)
			marker := "  "
			if i == m.historyCursor {
				marker = "▶ "
			}
			idxStr := tuiutil.Fit(fmt.Sprintf("%3d", i+1), idxW)
			atStr := tuiutil.Fit(e.endTime.Local().Format("2006-01-02 15:04:05"), atW)
			durStr := tuiutil.Fit(format.FormatDuration(msDur(e.elapsedMs)), durW)
			agoStr := tuiutil.Fit(formatAgo(ago), agoW)

			if i == m.historyCursor {
				rowPlain := tuiutil.Fit(marker+strings.Join([]string{idxStr, atStr, durStr, agoStr}, " "), innerW)
				lines = append(lines, lipgloss.NewStyle().Reverse(true).Render(rowPlain))
			} else {
				cells := strings.Join([]string{
					lipgloss.NewStyle().Foreground(colDarkGray).Render(idxStr),
					lipgloss.NewStyle().Foreground(logEntryColor(ago)).Bold(true).Render(atStr),
					lipgloss.NewStyle().Foreground(colWhite).Render(durStr),
					lipgloss.NewStyle().Foreground(colDarkGray).Render(agoStr),
				}, " ")
				lines = append(lines, marker+cells)
			}
		}
	}

	total := len(m.historyLogs)
	runWord := "runs"
	if total == 1 {
		runWord = "run"
	}
	right := fmt.Sprintf(" %d %s ", total, runWord)
	title := fmt.Sprintf(" Task History: %s ", m.historyTaskID)

	return tuiutil.Panel(m.width, mainH, title, right, colDarkGray, strings.Join(lines, "\n"))
}

func (m *model) logDeletePopup(now time.Time) string {
	displayTime := m.historyConfirmDelete
	if dt, err := time.Parse(time.RFC3339, m.historyConfirmDelete); err == nil {
		ago := now.Sub(dt.UTC())
		displayTime = fmt.Sprintf("%s (%s)", dt.Local().Format("2006-01-02 15:04:05"), formatAgo(ago))
	}
	content := fmt.Sprintf("Delete entry from %s?  [y] Yes   [n] No", displayTime)
	title := fmt.Sprintf(" Delete Log Entry — %s ", m.historyTaskID)
	return tuiutil.CenteredBox(title, colRed, content)
}

func msDur(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

func basicShortcuts(history bool) []tuiutil.Shortcut {
	if history {
		return []tuiutil.Shortcut{
			{Key: "↑↓/jk", Desc: "Navigate"},
			{Key: "?", Desc: "All Keys"},
			{Key: "q/Esc", Desc: "Back"},
		}
	}
	return []tuiutil.Shortcut{
		{Key: "↑↓/jk", Desc: "Navigate"},
		{Key: "Enter/h", Desc: "History"},
		{Key: "?", Desc: "All Keys"},
		{Key: "q/Esc", Desc: "Quit"},
	}
}

func allShortcuts(history bool) []tuiutil.Shortcut {
	if history {
		return []tuiutil.Shortcut{
			{Key: "↑↓/jk", Desc: "Navigate"},
			{Key: "PgUp/PgDn", Desc: "Page"},
			{Key: "d", Desc: "Delete Entry"},
			{Key: "r", Desc: "Refresh"},
			{Key: "?", Desc: "Help"},
			{Key: "q/Esc", Desc: "Back"},
		}
	}
	return []tuiutil.Shortcut{
		{Key: "↑↓/jk", Desc: "Navigate"},
		{Key: "PgUp/PgDn", Desc: "Page"},
		{Key: "←→/Tab", Desc: "Sort"},
		{Key: "s", Desc: "Toggle Order"},
		{Key: "Enter/h", Desc: "History"},
		{Key: "d", Desc: "Delete Task"},
		{Key: "r", Desc: "Refresh"},
		{Key: "?", Desc: "Help"},
		{Key: "q/Esc", Desc: "Quit"},
	}
}
