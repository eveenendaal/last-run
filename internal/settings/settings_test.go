package settings

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eveenendaal/last-run/internal/db"
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
	m := &model{db: database, state: stateList}
	m.reload()
	return m
}

// TestSettingsEditFlow drives the list -> edit -> save flow and ensures the
// log_retention validating setter persists a valid value.
func TestSettingsEditFlow(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	if v := m.View(); !strings.Contains(v, "Settings") {
		t.Errorf("missing settings title:\n%s", v)
	}

	// log_retention is always surfaced (default 30d).
	if len(m.settings) == 0 || m.settings[0].key != "log_retention" {
		t.Fatalf("expected log_retention row, got %+v", m.settings)
	}

	// Enter edit mode, render the popup.
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != stateEdit {
		t.Fatalf("expected stateEdit, got %v", m.state)
	}
	if v := m.View(); !strings.Contains(v, "Edit Setting") {
		t.Errorf("missing edit popup:\n%s", v)
	}

	// Clear the buffer and type a new valid value, then save.
	m.editBuf = ""
	for _, r := range "14d" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != stateList {
		t.Fatalf("expected stateList after save, got %v", m.state)
	}

	if val, ok, _ := db.GetSetting(m.db, "log_retention"); !ok || val != "14d" {
		t.Errorf("persisted retention = (%q, %v), want (14d, true)", val, ok)
	}

	// Help overlay renders.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if !m.showHelp || m.View() == "" {
		t.Fatal("help overlay did not render")
	}
}

// TestSettingsRejectsInvalid ensures invalid log_retention input keeps the
// editor open and does not persist.
func TestSettingsRejectsInvalid(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.editBuf = "not_a_duration"
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != stateEdit {
		t.Errorf("expected to stay in stateEdit on invalid input, got %v", m.state)
	}
}
