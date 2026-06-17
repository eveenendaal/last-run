package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eveenendaal/last-run/internal/db"
)

func testModel(t *testing.T) *model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "t.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.InitDB(database); err != nil {
		t.Fatal(err)
	}
	m := &model{db: database, dbPath: path, state: stateList}
	m.reload()
	return m
}

// TestSettingsEditFlow drives the list -> edit -> save flow for log_retention.
func TestSettingsEditFlow(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	if v := m.View(); !strings.Contains(v, "Settings") {
		t.Errorf("missing settings title:\n%s", v)
	}

	// With db_location now present, settings should have 2 rows sorted
	// alphabetically: db_location (index 0), log_retention (index 1).
	if len(m.settings) != 2 {
		t.Fatalf("expected 2 settings rows, got %d: %+v", len(m.settings), m.settings)
	}
	if m.settings[0].key != "db_location" {
		t.Errorf("settings[0] = %q, want db_location", m.settings[0].key)
	}
	if m.settings[1].key != "log_retention" {
		t.Errorf("settings[1] = %q, want log_retention", m.settings[1].key)
	}

	// Navigate to log_retention (index 1) and enter edit mode.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != stateEdit {
		t.Fatalf("expected stateEdit, got %v", m.state)
	}
	if m.editKey != "log_retention" {
		t.Errorf("editKey = %q, want log_retention", m.editKey)
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
	// Navigate to log_retention.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.editBuf = "not_a_duration"
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != stateEdit {
		t.Errorf("expected to stay in stateEdit on invalid input, got %v", m.state)
	}
}

func TestSettingsCursorNavigation(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Default cursor is at 0 (db_location).
	if m.cursor != 0 {
		t.Errorf("initial cursor = %d, want 0", m.cursor)
	}

	// Move down to log_retention.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.cursor != 1 {
		t.Errorf("cursor after down = %d, want 1", m.cursor)
	}

	// Move back up.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.cursor != 0 {
		t.Errorf("cursor after up = %d, want 0", m.cursor)
	}

	// Enter from log_retention position edits log_retention.
	m.cursor = 1
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.editKey != "log_retention" {
		t.Errorf("editKey = %q, want log_retention", m.editKey)
	}
}

func TestDBLocationRevert(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Edit db_location with an empty buffer → revert to default.
	m.cursor = 0
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.editKey != "db_location" {
		t.Fatalf("editKey = %q, want db_location", m.editKey)
	}
	m.editBuf = ""
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.state != stateList {
		t.Errorf("expected stateList after revert, got %v", m.state)
	}
	got, err := db.GetCustomDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("GetCustomDBPath = %q, want \"\" after revert", got)
	}
	if !strings.Contains(m.notice, "reverted") {
		t.Errorf("notice = %q, want mention of revert", m.notice)
	}
}

func TestDBLocationChooseAction_New(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	newPath := filepath.Join(t.TempDir(), "new.db")

	// Edit db_location, type a path that doesn't exist.
	m.cursor = 0
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.editBuf = newPath
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.state != stateChooseAction {
		t.Fatalf("expected stateChooseAction, got %v", m.state)
	}
	if m.targetExists {
		t.Error("targetExists should be false for a non-existent path")
	}
	if m.pendingPath != newPath {
		t.Errorf("pendingPath = %q, want %q", m.pendingPath, newPath)
	}

	// Choose "new".
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if m.state != stateList {
		t.Errorf("expected stateList after N, got %v", m.state)
	}
	got, err := db.GetCustomDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != newPath {
		t.Errorf("GetCustomDBPath = %q, want %q", got, newPath)
	}
}

func TestDBLocationChooseAction_SwitchExisting(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Create an existing DB file to switch to.
	existingPath := filepath.Join(t.TempDir(), "existing.db")
	if err := os.WriteFile(existingPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	m.cursor = 0
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.editBuf = existingPath
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.state != stateChooseAction {
		t.Fatalf("expected stateChooseAction, got %v", m.state)
	}
	if !m.targetExists {
		t.Error("targetExists should be true for an existing file")
	}

	// Choose "switch".
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if m.state != stateList {
		t.Errorf("expected stateList after S, got %v", m.state)
	}
	got, _ := db.GetCustomDBPath()
	if got != existingPath {
		t.Errorf("GetCustomDBPath = %q, want %q", got, existingPath)
	}
}

func TestDBLocationChooseAction_Cancel(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	newPath := filepath.Join(t.TempDir(), "cancel.db")
	m.cursor = 0
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.editBuf = newPath
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.state != stateChooseAction {
		t.Fatalf("expected stateChooseAction")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != stateList {
		t.Errorf("expected stateList after Esc, got %v", m.state)
	}
	if m.pendingPath != "" {
		t.Errorf("pendingPath = %q, want \"\" after cancel", m.pendingPath)
	}
}

func TestExportFlow(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	dstPath := filepath.Join(t.TempDir(), "export.db")

	// Press 'e' to enter export mode.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if m.state != stateEdit || m.editKey != "_export" {
		t.Fatalf("expected stateEdit with _export, got state=%v editKey=%q", m.state, m.editKey)
	}

	m.editBuf = dstPath
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != stateList {
		t.Errorf("expected stateList after export, got %v", m.state)
	}
	if _, err := os.Stat(dstPath); err != nil {
		t.Errorf("exported file not found at %s: %v", dstPath, err)
	}
	if !strings.Contains(m.notice, "Exported") {
		t.Errorf("notice = %q, want mention of Exported", m.notice)
	}
}

func TestImportFlow(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Create a fake DB file to import.
	srcPath := filepath.Join(t.TempDir(), "import_src.db")
	if err := os.WriteFile(srcPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	// Press 'i' to enter import mode.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if m.state != stateEdit || m.editKey != "_import" {
		t.Fatalf("expected stateEdit with _import, got state=%v editKey=%q", m.state, m.editKey)
	}

	m.editBuf = srcPath
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != stateList {
		t.Errorf("expected stateList after import, got %v", m.state)
	}
	got, _ := db.GetCustomDBPath()
	if got != srcPath {
		t.Errorf("GetCustomDBPath = %q, want %q", got, srcPath)
	}
}

func TestImportRejectsNonExistentFile(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m.editBuf = "/does/not/exist.db"
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// Should stay in editor since file doesn't exist.
	if m.state != stateEdit {
		t.Errorf("expected to stay in stateEdit for missing import file, got %v", m.state)
	}
}
