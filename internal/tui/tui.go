// Package tui implements the interactive `lastrun status` view using Bubble
// Tea. It renders a sortable task table, a per-task history drill-down with
// stats, delete-confirmation popups, and a help overlay, refreshing every
// 250ms so elapsed counters tick.
package tui

import (
	"database/sql"
	"fmt"
	"slices"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/eveenendaal/last-run/internal/db"
)

// SortCol identifies the column the task table is sorted by.
type SortCol int

const (
	SortTask SortCol = iota
	SortStatus
	SortDuration
	SortElapsed
	SortLastRun

	numSortCols = iota
)

const refreshInterval = 250 * time.Millisecond

type appState int

const (
	stateNormal appState = iota
	stateConfirmDelete
	stateHistory
)

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type model struct {
	db       *sql.DB
	idFilter *string

	tasks   []db.TaskStatus
	cursor  int
	sortCol SortCol
	sortAsc bool

	state           appState
	confirmDeleteID string

	historyTaskID        string
	historyLogs          []db.LogEntry
	historyCursor        int
	historyConfirmDelete string // raw end_time; "" means no confirmation pending

	showHelp    bool
	lastUpdated time.Time

	width           int
	height          int
	pageSize        int
	historyPageSize int

	err error
}

// RunTUI launches the interactive status view.
func RunTUI(database *sql.DB, idFilter *string, sortCol SortCol) error {
	m := &model{
		db:              database,
		idFilter:        idFilter,
		sortCol:         sortCol,
		sortAsc:         true,
		state:           stateNormal,
		pageSize:        10,
		historyPageSize: 10,
	}
	if err := m.loadTasks(); err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	if err != nil {
		return err
	}
	return m.err
}

func (m *model) Init() tea.Cmd {
	return tick()
}

// ── data loading ────────────────────────────────────────────────────────────

func (m *model) loadTasks() error {
	prevID := m.selectedID()
	tasks, err := db.GetAllTasks(m.db, m.idFilter)
	if err != nil {
		return err
	}
	m.tasks = tasks
	m.lastUpdated = time.Now().UTC()
	m.sortTasks()
	m.restoreCursor(prevID)
	return nil
}

// restoreCursor keeps the selection on prevID when it is still listed,
// otherwise clamps the cursor to the new list length.
func (m *model) restoreCursor(prevID string) {
	if i := slices.IndexFunc(m.tasks, func(t db.TaskStatus) bool { return t.ID == prevID }); prevID != "" && i >= 0 {
		m.cursor = i
		return
	}
	m.cursor = clampCursor(m.cursor, len(m.tasks))
}

// clampCursor keeps cursor within [0, n-1] (or 0 when n is 0).
func clampCursor(cursor, n int) int {
	return max(min(cursor, n-1), 0)
}

func (m *model) loadHistory(taskID string) error {
	m.historyTaskID = taskID
	m.historyLogs = nil
	m.historyCursor = 0
	return m.refreshHistory()
}

// refreshHistory reloads the history entries, keeping the selection on the
// same entry when it still exists.
func (m *model) refreshHistory() error {
	prevRaw := m.selectedHistoryRaw()
	entries, err := db.GetTaskLogEntries(m.db, m.historyTaskID)
	if err != nil {
		return err
	}
	m.historyLogs = entries
	if i := slices.IndexFunc(entries, func(e db.LogEntry) bool { return e.Raw == prevRaw }); prevRaw != "" && i >= 0 {
		m.historyCursor = i
	} else {
		m.historyCursor = clampCursor(m.historyCursor, len(entries))
	}
	return nil
}

// ── sorting ─────────────────────────────────────────────────────────────────

func (m *model) sortTasks() {
	now := time.Now().UTC()
	switch m.sortCol {
	case SortTask:
		sort.SliceStable(m.tasks, func(i, j int) bool { return m.tasks[i].ID < m.tasks[j].ID })
	case SortStatus:
		sort.SliceStable(m.tasks, func(i, j int) bool {
			return taskStatusOrder(m.tasks[i], now) < taskStatusOrder(m.tasks[j], now)
		})
	case SortDuration:
		sort.SliceStable(m.tasks, func(i, j int) bool { return nilLastLess(m.tasks[i].Duration, m.tasks[j].Duration, lessInt64) })
	case SortElapsed:
		sort.SliceStable(m.tasks, func(i, j int) bool {
			return elapsedMillis(m.tasks[i], now) < elapsedMillis(m.tasks[j], now)
		})
	case SortLastRun:
		sort.SliceStable(m.tasks, func(i, j int) bool { return nilLastLess(m.tasks[i].LastRun, m.tasks[j].LastRun, time.Time.Before) })
	}
	if !m.sortAsc {
		slices.Reverse(m.tasks)
	}
}

// nilLastLess orders set values before nil ones; among set values it defers
// to less.
func nilLastLess[T any](a, b *T, less func(T, T) bool) bool {
	if a == nil || b == nil {
		return a != nil && b == nil
	}
	return less(*a, *b)
}

func lessInt64(a, b int64) bool { return a < b }

func (m *model) cycleSortNext() {
	m.sortCol = (m.sortCol + 1) % numSortCols
	m.sortTasks()
}

func (m *model) cycleSortPrev() {
	m.sortCol = (m.sortCol + numSortCols - 1) % numSortCols
	m.sortTasks()
}

func (m *model) toggleSortOrder() {
	m.sortAsc = !m.sortAsc
	m.sortTasks()
}

// ── navigation ──────────────────────────────────────────────────────────────

func navUp(cursor, n int) int {
	if n == 0 {
		return 0
	}
	if cursor <= 0 {
		return n - 1
	}
	return cursor - 1
}

func navDown(cursor, n int) int {
	if n == 0 {
		return 0
	}
	if cursor+1 < n {
		return cursor + 1
	}
	return 0
}

func pageUp(cursor, page int) int {
	if page < 1 {
		page = 1
	}
	return max(cursor-page, 0)
}

func pageDown(cursor, page, n int) int {
	if n == 0 {
		return 0
	}
	if page < 1 {
		page = 1
	}
	return min(cursor+page, n-1)
}

func (m *model) selectedID() string {
	if m.cursor >= 0 && m.cursor < len(m.tasks) {
		return m.tasks[m.cursor].ID
	}
	return ""
}

func (m *model) selectedHistoryRaw() string {
	if m.historyCursor >= 0 && m.historyCursor < len(m.historyLogs) {
		return m.historyLogs[m.historyCursor].Raw
	}
	return ""
}

// ── update ──────────────────────────────────────────────────────────────────

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		switch m.state {
		case stateNormal, stateConfirmDelete:
			if err := m.loadTasks(); err != nil {
				m.err = err
				return m, tea.Quit
			}
		case stateHistory:
			if err := m.refreshHistory(); err != nil {
				m.err = err
				return m, tea.Quit
			}
		}
		return m, tick()
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.showHelp {
		switch key {
		case "?", "esc", "q":
			m.showHelp = false
		}
		return m, nil
	}

	switch m.state {
	case stateHistory:
		return m.handleHistoryKey(key)
	case stateNormal:
		return m.handleNormalKey(key)
	case stateConfirmDelete:
		return m.handleConfirmKey(key)
	}
	return m, nil
}

func (m *model) handleNormalKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.cursor = navUp(m.cursor, len(m.tasks))
	case "down", "j":
		m.cursor = navDown(m.cursor, len(m.tasks))
	case "pgup":
		m.cursor = pageUp(m.cursor, m.pageSize)
	case "pgdown":
		m.cursor = pageDown(m.cursor, m.pageSize, len(m.tasks))
	case "right", "tab":
		m.cycleSortNext()
	case "left", "shift+tab":
		m.cycleSortPrev()
	case "s":
		m.toggleSortOrder()
	case "r":
		if err := m.loadTasks(); err != nil {
			m.err = err
			return m, tea.Quit
		}
	case "d":
		if id := m.selectedID(); id != "" {
			m.state = stateConfirmDelete
			m.confirmDeleteID = id
		}
	case "enter", "h":
		if id := m.selectedID(); id != "" {
			if err := m.loadHistory(id); err != nil {
				m.err = err
				return m, tea.Quit
			}
			m.state = stateHistory
		}
	case "?":
		m.showHelp = true
	}
	return m, nil
}

func (m *model) handleConfirmKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "enter":
		if _, err := db.DeleteTaskLogs(m.db, m.confirmDeleteID); err != nil {
			m.err = err
			return m, tea.Quit
		}
		if _, err := db.DeleteTask(m.db, m.confirmDeleteID); err != nil {
			m.err = err
			return m, tea.Quit
		}
		m.state = stateNormal
		if err := m.loadTasks(); err != nil {
			m.err = err
			return m, tea.Quit
		}
	case "n", "esc", "q":
		m.state = stateNormal
	}
	return m, nil
}

func (m *model) handleHistoryKey(key string) (tea.Model, tea.Cmd) {
	if m.historyConfirmDelete != "" {
		switch key {
		case "y", "enter":
			raw := m.historyConfirmDelete
			m.historyConfirmDelete = ""
			if _, err := db.DeleteTaskLogEntry(m.db, m.historyTaskID, raw); err != nil {
				m.err = err
				return m, tea.Quit
			}
			if err := m.refreshHistory(); err != nil {
				m.err = err
				return m, tea.Quit
			}
		case "n", "esc", "q":
			m.historyConfirmDelete = ""
		}
		return m, nil
	}

	switch key {
	case "q", "esc":
		m.state = stateNormal
		m.historyLogs = nil
		m.historyTaskID = ""
	case "up", "k":
		m.historyCursor = navUp(m.historyCursor, len(m.historyLogs))
	case "down", "j":
		m.historyCursor = navDown(m.historyCursor, len(m.historyLogs))
	case "pgup":
		m.historyCursor = pageUp(m.historyCursor, m.historyPageSize)
	case "pgdown":
		m.historyCursor = pageDown(m.historyCursor, m.historyPageSize, len(m.historyLogs))
	case "d":
		if raw := m.selectedHistoryRaw(); raw != "" {
			m.historyConfirmDelete = raw
		}
	case "r":
		if err := m.refreshHistory(); err != nil {
			m.err = err
			return m, tea.Quit
		}
	case "?":
		m.showHelp = true
	}
	return m, nil
}

// ── status helpers ──────────────────────────────────────────────────────────

// statusOrder ranks statuses for the Status sort column.
var statusOrder = map[string]int{
	db.StatusOK:      0,
	db.StatusRunning: 1,
	db.StatusDue:     2,
	db.StatusUnknown: 3,
}

func taskStatusOrder(t db.TaskStatus, now time.Time) int {
	return statusOrder[t.Status(now)]
}

func elapsedMillis(t db.TaskStatus, now time.Time) int64 {
	d, _ := t.Elapsed(now)
	return d.Milliseconds()
}

func taskColor(t db.TaskStatus, now time.Time) lipgloss.Color {
	switch t.Status(now) {
	case db.StatusRunning:
		return colYellow
	case db.StatusDue:
		return colRed
	case db.StatusUnknown:
		return lipgloss.Color("4") // blue
	}
	if t.Duration == nil {
		return colWhite // ok, but no threshold to judge against
	}
	return colGreen
}

func logEntryColor(ago time.Duration) lipgloss.Color {
	secs := int64(ago.Seconds())
	switch {
	case secs < 3600:
		return lipgloss.Color("10") // light green
	case secs < 86400:
		return lipgloss.Color("2") // green
	case secs < 86400*7:
		return lipgloss.Color("3") // yellow
	default:
		return lipgloss.Color("8") // gray
	}
}

func formatAgo(ago time.Duration) string {
	secs := int64(ago.Seconds())
	if secs < 0 {
		secs = -secs
	}
	switch {
	case secs < 60:
		return fmt.Sprintf("%ds ago", secs)
	case secs < 3600:
		mm := secs / 60
		ss := secs % 60
		if ss == 0 {
			return fmt.Sprintf("%dm ago", mm)
		}
		return fmt.Sprintf("%dm%ds ago", mm, ss)
	case secs < 86400:
		hh := secs / 3600
		mm := (secs % 3600) / 60
		if mm == 0 {
			return fmt.Sprintf("%dh ago", hh)
		}
		return fmt.Sprintf("%dh%dm ago", hh, mm)
	default:
		dd := secs / 86400
		hh := (secs % 86400) / 3600
		if hh == 0 {
			return fmt.Sprintf("%dd ago", dd)
		}
		return fmt.Sprintf("%dd%dh ago", dd, hh)
	}
}

// historyStats returns (avgMs, minMs, maxMs, avgFreqMs, hasFreq, ok).
func (m *model) historyStats() (int64, int64, int64, int64, bool, bool) {
	n := len(m.historyLogs)
	if n == 0 {
		return 0, 0, 0, 0, false, false
	}
	var sum int64
	lo, hi := m.historyLogs[0].ElapsedMs, m.historyLogs[0].ElapsedMs
	for _, e := range m.historyLogs {
		sum += e.ElapsedMs
		lo = min(lo, e.ElapsedMs)
		hi = max(hi, e.ElapsedMs)
	}
	avg := sum / int64(n)
	if n >= 2 {
		newest := m.historyLogs[0].EndTime
		oldest := m.historyLogs[n-1].EndTime
		spanMs := newest.Sub(oldest).Milliseconds()
		return avg, lo, hi, spanMs / int64(n-1), true, true
	}
	return avg, lo, hi, 0, false, true
}
