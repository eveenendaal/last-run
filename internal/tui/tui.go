// Package tui implements the interactive `lastrun status` view using Bubble
// Tea, replacing the original ratatui implementation. It renders a sortable
// task table, a per-task history drill-down with stats, delete-confirmation
// popups, and a help overlay, refreshing every 250ms so elapsed counters tick.
package tui

import (
	"database/sql"
	"fmt"
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
)

const refreshInterval = 250 * time.Millisecond

type appState int

const (
	stateNormal appState = iota
	stateConfirmDelete
	stateHistory
)

type taskRow struct {
	id        string
	lastRun   *time.Time
	startTime *time.Time
	duration  *int64
}

type logEntry struct {
	raw       string
	endTime   time.Time
	elapsedMs int64
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type model struct {
	db       *sql.DB
	idFilter *string

	tasks   []taskRow
	cursor  int
	sortCol SortCol
	sortAsc bool

	state           appState
	confirmDeleteID string

	historyTaskID        string
	historyLogs          []logEntry
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
	rows, err := db.GetAllTasks(m.db, m.idFilter)
	if err != nil {
		return err
	}
	m.tasks = m.tasks[:0]
	for _, r := range rows {
		m.tasks = append(m.tasks, taskRow{id: r.ID, lastRun: r.LastRun, startTime: r.StartTime, duration: r.Duration})
	}
	m.lastUpdated = time.Now().UTC()
	m.sortTasks()
	m.restoreCursor(prevID)
	return nil
}

func (m *model) restoreCursor(prevID string) {
	if len(m.tasks) == 0 {
		m.cursor = 0
		return
	}
	if prevID != "" {
		for i, t := range m.tasks {
			if t.id == prevID {
				m.cursor = i
				return
			}
		}
	}
	if m.cursor >= len(m.tasks) {
		m.cursor = len(m.tasks) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *model) loadHistory(taskID string) error {
	entries, err := db.GetTaskLogEntries(m.db, taskID)
	if err != nil {
		return err
	}
	m.historyTaskID = taskID
	m.historyLogs = m.historyLogs[:0]
	for _, e := range entries {
		m.historyLogs = append(m.historyLogs, logEntry{raw: e.Raw, endTime: e.EndTime, elapsedMs: e.ElapsedMs})
	}
	m.historyCursor = 0
	if len(m.historyLogs) == 0 {
		m.historyCursor = 0
	}
	return nil
}

func (m *model) refreshHistory() error {
	prevRaw := m.selectedHistoryRaw()
	prevIdx := m.historyCursor
	entries, err := db.GetTaskLogEntries(m.db, m.historyTaskID)
	if err != nil {
		return err
	}
	m.historyLogs = m.historyLogs[:0]
	for _, e := range entries {
		m.historyLogs = append(m.historyLogs, logEntry{raw: e.Raw, endTime: e.EndTime, elapsedMs: e.ElapsedMs})
	}
	newIdx := -1
	if prevRaw != "" {
		for i, e := range m.historyLogs {
			if e.raw == prevRaw {
				newIdx = i
				break
			}
		}
	}
	if newIdx >= 0 {
		m.historyCursor = newIdx
	} else if len(m.historyLogs) == 0 {
		m.historyCursor = 0
	} else if prevIdx >= len(m.historyLogs) {
		m.historyCursor = len(m.historyLogs) - 1
	}
	return nil
}

// ── sorting ─────────────────────────────────────────────────────────────────

func (m *model) sortTasks() {
	now := time.Now().UTC()
	switch m.sortCol {
	case SortTask:
		sort.SliceStable(m.tasks, func(i, j int) bool { return m.tasks[i].id < m.tasks[j].id })
	case SortStatus:
		sort.SliceStable(m.tasks, func(i, j int) bool {
			return taskStatusOrder(m.tasks[i], now) < taskStatusOrder(m.tasks[j], now)
		})
	case SortDuration:
		sort.SliceStable(m.tasks, func(i, j int) bool { return durationLess(m.tasks[i].duration, m.tasks[j].duration) })
	case SortElapsed:
		sort.SliceStable(m.tasks, func(i, j int) bool {
			return elapsedMillis(m.tasks[i], now) < elapsedMillis(m.tasks[j], now)
		})
	case SortLastRun:
		sort.SliceStable(m.tasks, func(i, j int) bool { return timeLess(m.tasks[i].lastRun, m.tasks[j].lastRun) })
	}
	if !m.sortAsc {
		for i, j := 0, len(m.tasks)-1; i < j; i, j = i+1, j-1 {
			m.tasks[i], m.tasks[j] = m.tasks[j], m.tasks[i]
		}
	}
}

// durationLess orders Some before None; among Some, ascending by value.
func durationLess(a, b *int64) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil {
		return false // a (None) is greater
	}
	if b == nil {
		return true // a (Some) before b (None)
	}
	return *a < *b
}

// timeLess orders Some before None; among Some, ascending by time.
func timeLess(a, b *time.Time) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.Before(*b)
}

func (m *model) cycleSortNext() {
	m.sortCol = (m.sortCol + 1) % 5
	m.sortTasks()
}

func (m *model) cycleSortPrev() {
	m.sortCol = (m.sortCol + 4) % 5
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
	c := cursor - page
	if c < 0 {
		c = 0
	}
	return c
}

func pageDown(cursor, page, n int) int {
	if n == 0 {
		return 0
	}
	if page < 1 {
		page = 1
	}
	c := cursor + page
	if c > n-1 {
		c = n - 1
	}
	return c
}

func (m *model) selectedID() string {
	if m.cursor >= 0 && m.cursor < len(m.tasks) {
		return m.tasks[m.cursor].id
	}
	return ""
}

func (m *model) selectedHistoryRaw() string {
	if m.historyCursor >= 0 && m.historyCursor < len(m.historyLogs) {
		return m.historyLogs[m.historyCursor].raw
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

func taskStatusOrder(t taskRow, now time.Time) int {
	if t.startTime != nil && t.lastRun == nil {
		return 1
	}
	if t.lastRun != nil {
		if t.duration != nil {
			if now.Sub(*t.lastRun) > time.Duration(*t.duration)*time.Second {
				return 2
			}
			return 0
		}
		return 0
	}
	return 3
}

func elapsedMillis(t taskRow, now time.Time) int64 {
	switch {
	case t.startTime != nil && t.lastRun != nil && t.startTime.Before(*t.lastRun):
		return t.lastRun.Sub(*t.startTime).Milliseconds()
	case t.startTime != nil && t.lastRun == nil:
		return now.Sub(*t.startTime).Milliseconds()
	default:
		return 0
	}
}

func taskStatusStr(t taskRow, now time.Time) string {
	if t.startTime != nil && t.lastRun == nil {
		return "running"
	}
	if t.lastRun != nil {
		if t.duration != nil {
			if now.Sub(*t.lastRun) > time.Duration(*t.duration)*time.Second {
				return "due"
			}
			return "ok"
		}
		return "ok"
	}
	return "unknown"
}

func taskColor(t taskRow, now time.Time) lipgloss.Color {
	if t.startTime != nil && t.lastRun == nil {
		return lipgloss.Color("3") // yellow
	}
	if t.lastRun != nil {
		if t.duration != nil {
			if now.Sub(*t.lastRun) > time.Duration(*t.duration)*time.Second {
				return lipgloss.Color("1") // red
			}
			return lipgloss.Color("2") // green
		}
		return lipgloss.Color("15") // white
	}
	return lipgloss.Color("4") // blue
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
	min := m.historyLogs[0].elapsedMs
	max := m.historyLogs[0].elapsedMs
	for _, e := range m.historyLogs {
		sum += e.elapsedMs
		if e.elapsedMs < min {
			min = e.elapsedMs
		}
		if e.elapsedMs > max {
			max = e.elapsedMs
		}
	}
	avg := sum / int64(n)
	if n >= 2 {
		newest := m.historyLogs[0].endTime
		oldest := m.historyLogs[n-1].endTime
		spanMs := newest.Sub(oldest).Milliseconds()
		return avg, min, max, spanMs / int64(n-1), true, true
	}
	return avg, min, max, 0, false, true
}
