// Package tuiutil holds shared rendering primitives for the interactive views:
// titled panels, the bottom controls bar, the help modal, and a centered
// overlay compositor. Both the status and settings TUIs build on it.
package tuiutil

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Shortcut is a (key, description) pair shown in the controls bar / help modal.
type Shortcut struct {
	Key  string
	Desc string
}

var (
	borderColor = lipgloss.Color("8") // dark gray
	titleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	keyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("8")).Bold(true)
	descStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	sepStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// Fit truncates or right-pads a (possibly styled) string to exactly w display
// columns.
func Fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	sw := ansi.StringWidth(s)
	if sw == w {
		return s
	}
	if sw < w {
		return s + strings.Repeat(" ", w-sw)
	}
	return ansi.Truncate(s, w, "")
}

// Panel renders content inside a rounded border, embedding a left title and an
// optional right-aligned title in the top border (ratatui-style).
func Panel(width, height int, title, rightTitle string, border lipgloss.Color, content string) string {
	if width < 2 {
		width = 2
	}
	if height < 2 {
		height = 2
	}
	innerW := width - 2
	innerH := height - 2
	bs := lipgloss.NewStyle().Foreground(border)

	// Top border with embedded titles.
	lw := ansi.StringWidth(title)
	rw := ansi.StringWidth(rightTitle)
	if lw+rw > innerW {
		rightTitle = ""
		rw = 0
		title = ansi.Truncate(title, innerW, "")
		lw = ansi.StringWidth(title)
	}
	dashCount := innerW - lw - rw
	if dashCount < 0 {
		dashCount = 0
	}
	var top strings.Builder
	top.WriteString(bs.Render("╭"))
	top.WriteString(titleStyle.Render(title))
	top.WriteString(bs.Render(strings.Repeat("─", dashCount)))
	top.WriteString(titleStyle.Render(rightTitle))
	top.WriteString(bs.Render("╮"))

	// Body.
	contentLines := strings.Split(content, "\n")
	var b strings.Builder
	b.WriteString(top.String())
	b.WriteString("\n")
	for i := 0; i < innerH; i++ {
		line := ""
		if i < len(contentLines) {
			line = contentLines[i]
		}
		b.WriteString(bs.Render("│"))
		b.WriteString(Fit(line, innerW))
		b.WriteString(bs.Render("│"))
		b.WriteString("\n")
	}
	b.WriteString(bs.Render("╰" + strings.Repeat("─", innerW) + "╯"))
	return b.String()
}

// layoutControls splits shortcuts into wrapped lines of rendered spans and
// reports the number of text lines used.
func layoutControls(width int, shortcuts []Shortcut) ([]string, int) {
	contentWidth := width - 2
	if contentWidth < 1 {
		contentWidth = 1
	}

	var lines []string
	var cur strings.Builder
	cur.WriteString("  ")
	curWidth := 2
	firstOnLine := true

	flush := func() {
		lines = append(lines, cur.String())
		cur.Reset()
		cur.WriteString("  ")
		curWidth = 2
		firstOnLine = true
	}

	for _, sc := range shortcuts {
		keyW := ansi.StringWidth(sc.Key)
		descW := ansi.StringWidth(sc.Desc)
		sep := 0
		if !firstOnLine {
			sep = 2
		}
		item := sep + keyW + 2 + 1 + descW
		if !firstOnLine && curWidth+item > contentWidth {
			flush()
		}
		if !firstOnLine {
			cur.WriteString(sepStyle.Render("  "))
			curWidth += 2
		}
		cur.WriteString(keyStyle.Render(" " + sc.Key + " "))
		cur.WriteString(" ")
		cur.WriteString(descStyle.Render(sc.Desc))
		curWidth += keyW + 2 + 1 + descW
		firstOnLine = false
	}
	lines = append(lines, cur.String())
	return lines, len(lines)
}

// ControlsHeight returns the total height (including borders) of the controls
// bar for the given width and shortcuts.
func ControlsHeight(width int, shortcuts []Shortcut) int {
	_, n := layoutControls(width, shortcuts)
	return n + 2
}

// RenderControls renders the bottom controls bar as a bordered panel.
func RenderControls(width int, shortcuts []Shortcut) string {
	lines, n := layoutControls(width, shortcuts)
	return Panel(width, n+2, " Keys ", "", borderColor, strings.Join(lines, "\n"))
}

// RenderHelpModal renders the "All Keys" help modal box (unplaced).
func RenderHelpModal(shortcuts []Shortcut) string {
	maxKey := 4
	maxDesc := 4
	for _, sc := range shortcuts {
		if l := ansi.StringWidth(sc.Key) + 2; l > maxKey {
			maxKey = l
		}
		if l := ansi.StringWidth(sc.Desc); l > maxDesc {
			maxDesc = l
		}
	}
	innerW := maxKey + 1 + maxDesc + 4

	var lines []string
	for _, sc := range shortcuts {
		line := "  " + keyStyle.Render(" "+sc.Key+" ") + " " + descStyle.Render(sc.Desc)
		lines = append(lines, line)
	}

	white := lipgloss.Color("15")
	return Panel(innerW+2, len(shortcuts)+2, " All Keys ", " ? or Esc to close ", white, strings.Join(lines, "\n"))
}

// CenteredBox renders content in a rounded border with a colored title, sized
// to fit the content. Used for confirmation popups.
func CenteredBox(title string, titleColor lipgloss.Color, content string) string {
	w := ansi.StringWidth(content) + 4
	if tw := ansi.StringWidth(title) + 4; tw > w {
		w = tw
	}
	box := Panel(w, 3, title, "", titleColor, " "+content+" ")
	return box
}

// PlaceOverlay composites the foreground string centered over the background,
// preserving the background outside the foreground's footprint.
func PlaceOverlay(bg, fg string, totalW, totalH int) string {
	bgLines := strings.Split(bg, "\n")
	for len(bgLines) < totalH {
		bgLines = append(bgLines, "")
	}
	fgLines := strings.Split(fg, "\n")
	fgW := 0
	for _, l := range fgLines {
		if w := ansi.StringWidth(l); w > fgW {
			fgW = w
		}
	}
	fgH := len(fgLines)

	top := (totalH - fgH) / 2
	if top < 0 {
		top = 0
	}
	left := (totalW - fgW) / 2
	if left < 0 {
		left = 0
	}

	for i, fl := range fgLines {
		row := top + i
		if row < 0 || row >= len(bgLines) {
			continue
		}
		bl := bgLines[row]
		if w := ansi.StringWidth(bl); w < totalW {
			bl += strings.Repeat(" ", totalW-w)
		}
		leftPart := ansi.Truncate(bl, left, "")
		if w := ansi.StringWidth(leftPart); w < left {
			leftPart += strings.Repeat(" ", left-w)
		}
		rightPart := ansi.TruncateLeft(bl, left+fgW, "")
		flPadded := fl
		if w := ansi.StringWidth(fl); w < fgW {
			flPadded += strings.Repeat(" ", fgW-w)
		}
		bgLines[row] = leftPart + "\x1b[0m" + flPadded + "\x1b[0m" + rightPart
	}
	return strings.Join(bgLines, "\n")
}
