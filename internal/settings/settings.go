// Package settings implements the interactive `lastrun settings` editor using
// Bubble Tea, replacing the original ratatui implementation. It lists settings
// and lets the user edit them, routing log_retention through the validating
// setter so invalid input is rejected.
package settings

import (
	"database/sql"
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
)

type kv struct {
	key   string
	value string
}

type model struct {
	db       *sql.DB
	settings []kv
	state    settingsState
	editKey  string
	editBuf  string
	showHelp bool
	width    int
	height   int
	err      error
}

// RunSettingsTUI launches the interactive settings editor.
func RunSettingsTUI(database *sql.DB) error {
	m := &model{db: database, state: stateList}
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
		sort.Slice(m.settings, func(i, j int) bool { return m.settings[i].key < m.settings[j].key })
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
		switch key {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "enter":
			if len(m.settings) > 0 {
				m.editKey = m.settings[0].key
				m.editBuf = m.settings[0].value
				m.state = stateEdit
			}
		case "?":
			m.showHelp = true
		}
	case stateEdit:
		switch key {
		case "enter":
			saved := m.save()
			if saved {
				m.reload()
				m.state = stateList
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
	}
	return m, nil
}

// save persists the current edit buffer, routing log_retention through the
// validating setter. Returns false if validation failed (stay in editor).
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
)

func (m *model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	shortcuts := basicShortcuts(m.state)
	ctrlH := tuiutil.ControlsHeight(m.width, shortcuts)
	mainH := m.height - ctrlH
	if mainH < 3 {
		mainH = 3
	}

	base := m.renderTable(mainH) + "\n" + tuiutil.RenderControls(m.width, shortcuts)

	if m.state == stateEdit {
		box := tuiutil.CenteredBox(
			" Edit Setting — "+m.editKey+" ",
			colYellow,
			"Edit "+m.editKey+": "+m.editBuf+"█",
		)
		base = tuiutil.PlaceOverlay(base, box, m.width, m.height)
	}
	if m.showHelp {
		base = tuiutil.PlaceOverlay(base, tuiutil.RenderHelpModal(allShortcuts(m.state)), m.width, m.height)
	}
	return base
}

func (m *model) renderTable(mainH int) string {
	innerW := m.width - 2
	keyW := innerW / 2
	if keyW < 5 {
		keyW = 5
	}
	valW := innerW - keyW - 1
	if valW < 5 {
		valW = 5
	}

	hStyle := lipgloss.NewStyle().Foreground(colDarkGray).Bold(true)
	header := hStyle.Render(tuiutil.Fit("Key", keyW)) + " " + hStyle.Render(tuiutil.Fit("Value", valW))
	lines := []string{header, ""}

	if len(m.settings) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(colDarkGray).Render("No settings configured."))
	} else {
		for _, s := range m.settings {
			keyCell := lipgloss.NewStyle().Foreground(colWhite).Render(tuiutil.Fit(s.key, keyW))
			valCell := lipgloss.NewStyle().Foreground(colYellow).Render(tuiutil.Fit(s.value, valW))
			lines = append(lines, keyCell+" "+valCell)
		}
	}

	return tuiutil.Panel(m.width, mainH, " Settings ", "", colDarkGray, strings.Join(lines, "\n"))
}

func basicShortcuts(state settingsState) []tuiutil.Shortcut {
	if state == stateEdit {
		return []tuiutil.Shortcut{{Key: "Enter", Desc: "Save"}, {Key: "Esc", Desc: "Cancel"}}
	}
	return []tuiutil.Shortcut{{Key: "Enter", Desc: "Edit"}, {Key: "?", Desc: "All Keys"}, {Key: "q/Esc", Desc: "Quit"}}
}

func allShortcuts(state settingsState) []tuiutil.Shortcut {
	if state == stateEdit {
		return []tuiutil.Shortcut{
			{Key: "Enter", Desc: "Save"},
			{Key: "Esc", Desc: "Cancel"},
			{Key: "Backspace", Desc: "Delete"},
		}
	}
	return []tuiutil.Shortcut{
		{Key: "Enter", Desc: "Edit Setting"},
		{Key: "?", Desc: "Help"},
		{Key: "q/Esc", Desc: "Quit"},
	}
}
