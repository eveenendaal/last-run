package tui

import (
	"testing"
	"time"

	"github.com/eveenendaal/last-run/internal/db"
)

func TestNavigation(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"up wraps to end", navUp(0, 5), 4},
		{"up", navUp(3, 5), 2},
		{"up empty", navUp(0, 0), 0},
		{"down wraps to start", navDown(4, 5), 0},
		{"down", navDown(1, 5), 2},
		{"down empty", navDown(0, 0), 0},
		{"page up clamps", pageUp(3, 10), 0},
		{"page up", pageUp(15, 10), 5},
		{"page down clamps", pageDown(3, 10, 5), 4},
		{"page down", pageDown(0, 3, 10), 3},
		{"page down empty", pageDown(0, 10, 0), 0},
		{"clamp high", clampCursor(9, 3), 2},
		{"clamp empty", clampCursor(2, 0), 0},
		{"clamp negative", clampCursor(-1, 3), 0},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestSortTasks(t *testing.T) {
	now := time.Now().UTC()
	at := func(d time.Duration) *time.Time { return new(now.Add(-d)) }
	hour := int64(3600)
	m := &model{tasks: []db.TaskStatus{
		{ID: "c-unknown"},
		{ID: "a-due", LastRun: at(2 * time.Hour), Duration: &hour},
		{ID: "b-running", StartTime: at(time.Minute)},
		{ID: "d-ok", StartTime: at(time.Hour + time.Second), LastRun: at(time.Hour)},
	}}

	ids := func() []string {
		out := make([]string, len(m.tasks))
		for i, t := range m.tasks {
			out[i] = t.ID
		}
		return out
	}
	check := func(col SortCol, asc bool, want ...string) {
		t.Helper()
		m.sortCol, m.sortAsc = col, asc
		m.sortTasks()
		got := ids()
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("sort %v asc=%v = %v, want %v", col, asc, got, want)
				return
			}
		}
	}

	check(SortTask, true, "a-due", "b-running", "c-unknown", "d-ok")
	check(SortTask, false, "d-ok", "c-unknown", "b-running", "a-due")
	check(SortStatus, true, "d-ok", "b-running", "a-due", "c-unknown")
	// Tasks without a value sort last.
	check(SortDuration, true, "a-due")
	check(SortLastRun, true, "a-due", "d-ok")
	check(SortElapsed, false, "b-running", "d-ok")
}

func TestCycleSort(t *testing.T) {
	m := &model{sortCol: SortLastRun}
	m.cycleSortNext()
	if m.sortCol != SortTask {
		t.Errorf("next after last-run = %v, want task", m.sortCol)
	}
	m.cycleSortPrev()
	if m.sortCol != SortLastRun {
		t.Errorf("prev after task = %v, want last-run", m.sortCol)
	}
}

func TestFormatAgo(t *testing.T) {
	cases := map[time.Duration]string{
		45 * time.Second:              "45s ago",
		5 * time.Minute:               "5m ago",
		5*time.Minute + 3*time.Second: "5m3s ago",
		2 * time.Hour:                 "2h ago",
		2*time.Hour + 15*time.Minute:  "2h15m ago",
		3 * 24 * time.Hour:            "3d ago",
		3*24*time.Hour + 4*time.Hour:  "3d4h ago",
		-30 * time.Second:             "30s ago",
	}
	for in, want := range cases {
		if got := formatAgo(in); got != want {
			t.Errorf("formatAgo(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestHistoryStats(t *testing.T) {
	m := &model{}
	if _, _, _, _, _, ok := m.historyStats(); ok {
		t.Error("empty history should report ok=false")
	}

	end := time.Now().UTC()
	m.historyLogs = []db.LogEntry{ // newest first
		{EndTime: end, ElapsedMs: 3000},
		{EndTime: end.Add(-2 * time.Hour), ElapsedMs: 1000},
		{EndTime: end.Add(-4 * time.Hour), ElapsedMs: 2000},
	}
	avg, lo, hi, freq, hasFreq, ok := m.historyStats()
	if !ok || avg != 2000 || lo != 1000 || hi != 3000 {
		t.Errorf("stats = avg %d min %d max %d ok %v, want 2000/1000/3000", avg, lo, hi, ok)
	}
	if !hasFreq || freq != (2*time.Hour).Milliseconds() {
		t.Errorf("freq = %d (has %v), want 2h", freq, hasFreq)
	}

	m.historyLogs = m.historyLogs[:1]
	if _, _, _, _, hasFreq, ok := m.historyStats(); !ok || hasFreq {
		t.Errorf("single entry: ok=%v hasFreq=%v, want true/false", ok, hasFreq)
	}
}
