package tuiutil

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestFit(t *testing.T) {
	cases := []struct {
		in   string
		w    int
		want string
	}{
		{"abc", 5, "abc  "},
		{"abcdef", 3, "abc"},
		{"abc", 3, "abc"},
		{"abc", 0, ""},
	}
	for _, c := range cases {
		if got := Fit(c.in, c.w); got != c.want {
			t.Errorf("Fit(%q, %d) = %q, want %q", c.in, c.w, got, c.want)
		}
	}

	styled := lipgloss.NewStyle().Bold(true).Render("hello")
	if w := ansi.StringWidth(Fit(styled, 8)); w != 8 {
		t.Errorf("Fit(styled, 8) width = %d, want 8", w)
	}
}

func TestPanel(t *testing.T) {
	out := Panel(20, 4, " Title ", " R ", lipgloss.Color("8"), "one\ntwo\nthree")
	lines := strings.Split(ansi.Strip(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("Panel height = %d lines, want 4:\n%s", len(lines), out)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 20 {
			t.Errorf("line %d width = %d, want 20: %q", i, w, l)
		}
	}
	if !strings.HasPrefix(lines[0], "╭ Title ") || !strings.HasSuffix(lines[0], " R ╮") {
		t.Errorf("top border = %q", lines[0])
	}
	if !strings.Contains(lines[1], "one") || !strings.Contains(lines[2], "two") {
		t.Errorf("body = %q", lines[1:3])
	}
	// Content beyond the inner height is dropped.
	if strings.Contains(ansi.Strip(out), "three") {
		t.Errorf("overflowing content should be clipped:\n%s", out)
	}

	// A right title that doesn't fit is dropped rather than overflowing.
	narrow := ansi.Strip(Panel(10, 2, " Long Title ", " Right ", lipgloss.Color("8"), ""))
	if strings.Contains(narrow, "Right") || ansi.StringWidth(strings.Split(narrow, "\n")[0]) != 10 {
		t.Errorf("narrow panel = %q", narrow)
	}
}

func TestControlsWrap(t *testing.T) {
	shortcuts := []Shortcut{{"a", "Alpha"}, {"b", "Bravo"}, {"c", "Charlie"}, {"d", "Delta"}}
	if h := ControlsHeight(200, shortcuts); h != 3 {
		t.Errorf("wide ControlsHeight = %d, want 3 (one line + borders)", h)
	}
	narrowH := ControlsHeight(20, shortcuts)
	if narrowH <= 3 {
		t.Errorf("narrow ControlsHeight = %d, want wrapping onto several lines", narrowH)
	}
	if lines := strings.Split(RenderControls(20, shortcuts), "\n"); len(lines) != narrowH {
		t.Errorf("RenderControls height = %d, want %d", len(lines), narrowH)
	}
}

func TestRenderHelpModal(t *testing.T) {
	out := ansi.Strip(RenderHelpModal([]Shortcut{{"q", "Quit"}, {"?", "Help"}}))
	for _, want := range []string{"All Keys", "Quit", "Help"} {
		if !strings.Contains(out, want) {
			t.Errorf("help modal missing %q:\n%s", want, out)
		}
	}
}

func TestPlaceOverlay(t *testing.T) {
	bg := strings.Repeat(".........\n", 4) + "........."
	box := CenteredBox(" X ", lipgloss.Color("1"), "hi")
	out := ansi.Strip(PlaceOverlay(bg, box, 9, 5))
	lines := strings.Split(out, "\n")
	if len(lines) != 5 {
		t.Fatalf("overlay height = %d, want 5", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 9 {
			t.Errorf("line %d width = %d, want 9: %q", i, w, l)
		}
	}
	// The 3-line box is centered vertically: rows 1..3 hold it, 0 and 4 are untouched.
	if lines[0] != "........." || lines[4] != "........." {
		t.Errorf("rows outside the box changed:\n%s", out)
	}
	if !strings.Contains(lines[2], "hi") || !strings.HasPrefix(lines[2], ".") {
		t.Errorf("middle row = %q, want box content with background margin", lines[2])
	}
}
