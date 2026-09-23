// Package settings implements the interactive `lastrun settings` editor using
// Bubble Tea. It lists settings and lets the user edit them, routing
// log_retention through the validating setter so invalid input is rejected. It
// also provides import/export and custom database-location management.
package settings

import (
	"database/sql"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/eveenendaal/last-run/internal/db"
	"github.com/eveenendaal/last-run/internal/tuiutil"
)

type settingsState int

const (
	stateList settingsState = iota
	stateEdit
	stateChooseAction // shown after typing a new db_location path
)

type kv struct {
	key   string
	value string
}

type model struct {
	db           *sql.DB
	dbPath       string // resolved path of the currently-open database
	settings     []kv
	state        settingsState
	cursor       int    // selected row in stateList
	editKey      string // "_export" / "_import" are sentinel values
	editBuf      string
	pendingPath  string // new DB path staged for stateChooseAction
	targetExists bool   // whether pendingPath already has a file on disk
	notice       string // transient status message shown in the table
	showHelp     bool
	width        int
	height       int
	err          error
}

// RunSettingsTUI launches the interactive settings editor.
// dbPath is the resolved path of the currently-open database, used to display
// the current location and as the migration source.
func RunSettingsTUI(database *sql.DB, dbPath string) error {
	m := &model{db: database, dbPath: dbPath, state: stateList}
	m.reload()
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	if err != nil {
		return err
	}
	return m.err
}

func (m *model) reload() {
	rows, err := db.GetAllSettings(m.db)
	if err != nil {
		rows = nil
	}
	m.settings = m.settings[:0]
	hasRetention := false
	for _, r := range rows {
		if r.Key == "log_retention" {
			hasRetention = true
		}
		m.settings = append(m.settings, kv{key: r.Key, value: r.Value})
	}
	// Always surface log_retention so it can be edited even on a fresh DB; this
	// row is display-only ("30d" default) until saved.
	if !hasRetention {
		m.settings = append(m.settings, kv{key: "log_retention", value: "30d"})
	}
	// Always surface db_location using the currently-open DB path.
	m.settings = append(m.settings, kv{key: "db_location", value: m.dbPath})
	sort.Slice(m.settings, func(i, j int) bool { return m.settings[i].key < m.settings[j].key })
	// Clamp cursor after a reload that may have changed row count.
	if m.cursor >= len(m.settings) && len(m.settings) > 0 {
		m.cursor = len(m.settings) - 1
	}
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
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
	case stateList:
		if m.notice != "" && key != "?" {
			m.notice = ""
		}
		switch key {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.settings)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.settings) > 0 && m.cursor < len(m.settings) {
				m.editKey = m.settings[m.cursor].key
				m.editBuf = m.settings[m.cursor].value
				m.state = stateEdit
			}
		case "e":
			m.editKey = "_export"
			m.editBuf = ""
			m.state = stateEdit
		case "i":
			m.editKey = "_import"
			m.editBuf = ""
			m.state = stateEdit
		case "?":
			m.showHelp = true
		}

	case stateEdit:
		switch key {
		case "enter":
			newVal := strings.TrimSpace(m.editBuf)
			switch m.editKey {

			case "_export":
				if newVal == "" {
					// stay in editor until user provides a path
					break
				}
				if err := db.CopyDatabase(m.db, newVal); err != nil {
					m.finish("Export failed: " + err.Error())
				} else {
					m.finish("Exported to " + newVal)
				}

			case "_import":
				if newVal == "" {
					break
				}
				if _, err := os.Stat(newVal); err != nil {
					// File must exist to import — stay in editor.
					break
				}
				m.useDBPath(newVal, "Will use "+newVal+" on next restart")

			case "db_location":
				if newVal == "" {
					// Empty = revert to XDG default.
					m.useDBPath("", "DB location reverted to default (restart to apply)")
				} else if newVal == m.dbPath {
					m.state = stateList
				} else {
					_, statErr := os.Stat(newVal)
					m.targetExists = statErr == nil
					m.pendingPath = newVal
					m.state = stateChooseAction
				}

			default:
				if m.save() {
					m.reload()
					m.state = stateList
				}
			}

		case "esc":
			m.state = stateList
		case "backspace":
			if len(m.editBuf) > 0 {
				r := []rune(m.editBuf)
				m.editBuf = string(r[:len(r)-1])
			}
		default:
			if len(msg.Runes) == 1 {
				m.editBuf += string(msg.Runes)
			}
		}

	case stateChooseAction:
		switch key {
		case "s", "S":
			if m.targetExists {
				m.useDBPath(m.pendingPath, "Will switch to existing DB at "+m.pendingPath+" on next restart")
			}

		case "m", "M":
			m.migrate()

		case "n", "N":
			if !m.targetExists { // "new" only offered when target doesn't exist
				m.useDBPath(m.pendingPath, "New empty DB will be created at "+m.pendingPath+" on next restart")
			}

		case "esc", "ctrl+c":
			m.pendingPath = ""
			m.state = stateList
		}
	}
	return m, nil
}

// finish shows notice, discards any staged path, and returns to the list.
func (m *model) finish(notice string) {
	m.notice = notice
	m.pendingPath = ""
	m.reload()
	m.state = stateList
}

// useDBPath records path as the database location for the next start ("" to
// revert to the default) and finishes with okNotice on success.
func (m *model) useDBPath(path, okNotice string) {
	if err := db.SetCustomDBPath(path); err != nil {
		m.finish("Config write failed: " + err.Error())
		return
	}
	m.finish(okNotice)
}

// migrate copies the live database to the staged path (replacing any file
// already there) and points the config at it.
func (m *model) migrate() {
	path := m.pendingPath
	if m.targetExists {
		// Remove existing file so VACUUM INTO can write it.
		if err := os.Remove(path); err != nil {
			m.finish("Could not remove existing file: " + err.Error())
			return
		}
	}
	if err := db.CopyDatabase(m.db, path); err != nil {
		m.finish("Migration failed: " + err.Error())
		return
	}
	if err := db.SetCustomDBPath(path); err != nil {
		m.finish("Migrated but config write failed: " + err.Error())
		return
	}
	m.finish("Migrated to " + path + " (restart to apply)")
}

// save persists the current edit buffer for non-special settings, routing
// log_retention through the validating setter. Returns false to stay in editor.
func (m *model) save() bool {
	if m.editKey == "log_retention" {
		return db.SetLogRetention(m.db, strings.TrimSpace(m.editBuf)) == nil
	}
	return db.SetSetting(m.db, m.editKey, m.editBuf) == nil
}

var (
	colDarkGray = lipgloss.Color("8")
	colWhite    = lipgloss.Color("15")
	colYellow   = lipgloss.Color("3")
	colGreen    = lipgloss.Color("2")
)

func (m *model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	shortcuts := basicShortcuts(m.state)
	ctrlH := tuiutil.ControlsHeight(m.width, shortcuts)
	mainH := max(m.height-ctrlH, 3)

	base := m.renderTable(mainH) + "\n" + tuiutil.RenderControls(m.width, shortcuts)

	if m.state == stateChooseAction {
		box := m.renderChoiceBox()
		base = tuiutil.PlaceOverlay(base, box, m.width, m.height)
	}

	if m.state == stateEdit {
		title, prompt := m.editTitlePrompt()
		box := tuiutil.CenteredBox(title, colYellow, prompt)
		base = tuiutil.PlaceOverlay(base, box, m.width, m.height)
	}

	if m.showHelp {
		base = tuiutil.PlaceOverlay(base, tuiutil.RenderHelpModal(allShortcuts(m.state)), m.width, m.height)
	}
	return base
}

func (m *model) editTitlePrompt() (title, prompt string) {
	switch m.editKey {
	case "_export":
		return " Export Database ", "Destination path: " + m.editBuf + "█"
	case "_import":
		return " Import Database ", "Source path: " + m.editBuf + "█"
	default:
		return " Edit Setting — " + m.editKey + " ", "Edit " + m.editKey + ": " + m.editBuf + "█"
	}
}

func (m *model) renderChoiceBox() string {
	pathDisplay := m.pendingPath
	if len(pathDisplay) > 52 {
		pathDisplay = "..." + pathDisplay[len(pathDisplay)-49:]
	}

	var lines []string
	lines = append(lines, "  Path: "+pathDisplay)
	if m.targetExists {
		lines = append(lines, "  (file already exists at this location)", "")
		lines = append(lines, "  [S] Switch to existing database")
		lines = append(lines, "  [M] Migrate — overwrite with current data")
	} else {
		lines = append(lines, "")
		lines = append(lines, "  [M] Migrate — copy current database here")
		lines = append(lines, "  [N] New — start with an empty database")
	}
	lines = append(lines, "  [Esc] Cancel")

	return tuiutil.Panel(62, len(lines)+2, " Change Database Location ", "", colYellow,
		strings.Join(lines, "\n"))
}

func (m *model) renderTable(mainH int) string {
	innerW := m.width - 2
	keyW := max(innerW/3, 10)
	valW := max(innerW-keyW-1, 5)

	hStyle := lipgloss.NewStyle().Foreground(colDarkGray).Bold(true)
	header := hStyle.Render(tuiutil.Fit("Key", keyW)) + " " + hStyle.Render(tuiutil.Fit("Value", valW))
	lines := []string{header, ""}

	if len(m.settings) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(colDarkGray).Render("No settings configured."))
	} else {
		for i, s := range m.settings {
			if i == m.cursor {
				sel := lipgloss.NewStyle().Bold(true).Reverse(true)
				line := tuiutil.Fit(s.key, keyW) + " " + tuiutil.Fit(s.value, valW)
				lines = append(lines, sel.Render(tuiutil.Fit(line, innerW)))
			} else {
				keyCell := lipgloss.NewStyle().Foreground(colWhite).Render(tuiutil.Fit(s.key, keyW))
				valCell := lipgloss.NewStyle().Foreground(colYellow).Render(tuiutil.Fit(s.value, valW))
				lines = append(lines, keyCell+" "+valCell)
			}
		}
	}

	if m.notice != "" {
		lines = append(lines, "")
		noticeStyle := lipgloss.NewStyle().Foreground(colGreen).Italic(true)
		lines = append(lines, noticeStyle.Render(tuiutil.Fit(m.notice, innerW)))
	}

	return tuiutil.Panel(m.width, mainH, " Settings ", "", colDarkGray, strings.Join(lines, "\n"))
}

func basicShortcuts(state settingsState) []tuiutil.Shortcut {
	switch state {
	case stateEdit:
		return []tuiutil.Shortcut{{Key: "Enter", Desc: "Save"}, {Key: "Esc", Desc: "Cancel"}}
	case stateChooseAction:
		return []tuiutil.Shortcut{{Key: "Esc", Desc: "Cancel"}}
	default:
		return []tuiutil.Shortcut{
			{Key: "↑↓/jk", Desc: "Navigate"},
			{Key: "Enter", Desc: "Edit"},
			{Key: "e", Desc: "Export DB"},
			{Key: "i", Desc: "Import DB"},
			{Key: "?", Desc: "All Keys"},
			{Key: "q/Esc", Desc: "Quit"},
		}
	}
}

func allShortcuts(state settingsState) []tuiutil.Shortcut {
	switch state {
	case stateEdit:
		return []tuiutil.Shortcut{
			{Key: "Enter", Desc: "Save"},
			{Key: "Esc", Desc: "Cancel"},
			{Key: "Backspace", Desc: "Delete"},
		}
	case stateChooseAction:
		return []tuiutil.Shortcut{
			{Key: "S", Desc: "Switch (existing file)"},
			{Key: "M", Desc: "Migrate"},
			{Key: "N", Desc: "New empty DB"},
			{Key: "Esc", Desc: "Cancel"},
		}
	default:
		return []tuiutil.Shortcut{
			{Key: "↑↓/jk", Desc: "Navigate"},
			{Key: "Enter", Desc: "Edit Setting"},
			{Key: "e", Desc: "Export DB"},
			{Key: "i", Desc: "Import DB"},
			{Key: "?", Desc: "Help"},
			{Key: "q/Esc", Desc: "Quit"},
		}
	}
}
