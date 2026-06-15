package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eveenendaal/last-run/internal/db"
	taskmodel "github.com/eveenendaal/last-run/internal/model"
)

func testModel(t *testing.T) *model {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.InitDB(database); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	running := &taskmodel.Task{ID: "alpha", StartTime: &now}
	if err := running.Insert(database); err != nil {
		t.Fatal(err)
	}
	beta := &taskmodel.Task{ID: "beta"}
	if err := beta.Insert(database); err != nil {
		t.Fatal(err)
	}
	start := now.Add(-time.Hour)
	last := now.Add(-time.Minute)
	beta.StartTime = &start
	beta.LastRun = &last
	if err := beta.Update(database); err != nil {
		t.Fatal(err)
	}
	_ = db.UpdateTaskDuration(database, "beta", 3600)

	m := &model{db: database, sortCol: SortLastRun, sortAsc: true, state: stateNormal, pageSize: 10, historyPageSize: 10}
	if err := m.loadTasks(); err != nil {
		t.Fatal(err)
	}
	return m
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// TestViewRendersAllStates walks the major states and renders after each,
// guarding against panics in the panel/overlay rendering paths.
func TestViewRendersAllStates(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	if v := m.View(); !strings.Contains(v, "Last Run Status") {
		t.Errorf("normal view missing title:\n%s", v)
	}

	// Navigation + sorting must not panic and keep rendering.
	for _, k := range []string{"j", "k", "tab", "s", "pgdown", "pgup"} {
		m.Update(key(k))
		if m.View() == "" {
			t.Fatalf("empty view after key %q", k)
		}
	}

	// Confirm-delete popup.
	m.Update(key("d"))
	if m.state != stateConfirmDelete {
		t.Fatalf("expected stateConfirmDelete, got %v", m.state)
	}
	if v := m.View(); !strings.Contains(v, "Confirm Delete Task") {
		t.Errorf("missing confirm popup:\n%s", v)
	}
	m.Update(key("n"))
	if m.state != stateNormal {
		t.Fatalf("expected stateNormal after cancel, got %v", m.state)
	}

	// History view + log-delete popup + help overlay.
	for i, tk := range m.tasks {
		if tk.id == "beta" { // beta has a log entry
			m.cursor = i
		}
	}
	m.Update(key("enter"))
	if m.state != stateHistory {
		t.Fatalf("expected stateHistory, got %v", m.state)
	}
	if v := m.View(); !strings.Contains(v, "Task History") {
		t.Errorf("missing history view:\n%s", v)
	}
	m.Update(key("d")) // request delete of selected log entry
	if v := m.View(); !strings.Contains(v, "Delete Log Entry") {
		t.Errorf("missing log-delete popup:\n%s", v)
	}
	m.Update(key("n"))
	m.Update(key("?"))
	if !m.showHelp {
		t.Fatal("expected help overlay")
	}
	if m.View() == "" {
		t.Fatal("empty help view")
	}
	m.Update(key("?"))
	m.Update(key("esc")) // back to normal
	if m.state != stateNormal {
		t.Fatalf("expected stateNormal after history esc, got %v", m.state)
	}
}

// TestViewSmallTerminal exercises the narrow-width layout branch.
func TestViewSmallTerminal(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	if m.View() == "" {
		t.Fatal("empty view at small size")
	}
}
