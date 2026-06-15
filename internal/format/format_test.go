package format

import (
	"strings"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"5h", 5 * time.Hour},
		{"24h", 24 * time.Hour},
		{"0h", 0},
		{"1d", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
		{"1w", 7 * 24 * time.Hour},
		{"2w", 14 * 24 * time.Hour},
		{"1m", 30 * 24 * time.Hour},
		{"3m", 90 * 24 * time.Hour},
	}
	for _, c := range cases {
		got, err := ParseDuration(c.in)
		if err != nil {
			t.Errorf("ParseDuration(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", c.in, got, c.want)
		}
	}

	for _, bad := range []string{"5", "ab", ""} {
		if _, err := ParseDuration(bad); err == nil {
			t.Errorf("ParseDuration(%q) expected error, got nil", bad)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{5 * time.Minute, "5m0s"},
		{65 * time.Minute, "1h5m"},
		{5 * time.Hour, "5h0m"},
		{25 * time.Hour, "1d1h"},
		{24 * time.Hour, "1d0h"},
		{2*24*time.Hour + 12*time.Hour + 30*time.Minute, "2d12h"},
		{1500 * time.Millisecond, "1.50s"},
		{500 * time.Millisecond, "0.50s"},
		{10 * time.Millisecond, "0.01s"},
		{1 * time.Millisecond, "0.00s"},
		{5*time.Second + 250*time.Millisecond, "5.25s"},
		{60*time.Second + 750*time.Millisecond, "1m0s"},
	}
	for _, c := range cases {
		if got := FormatDuration(c.in); got != c.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatDatetime(t *testing.T) {
	testDt := time.Date(2023, 5, 15, 12, 0, 0, 0, time.UTC)
	formatted := FormatDatetime(testDt)

	if len(formatted) < 19 {
		t.Errorf("FormatDatetime length = %d, want >= 19 (%q)", len(formatted), formatted)
	}
	if !strings.HasPrefix(formatted, "2023-05-15") {
		t.Errorf("FormatDatetime = %q, want prefix 2023-05-15", formatted)
	}
	if !strings.Contains(formatted, ":") {
		t.Errorf("FormatDatetime = %q, want to contain ':'", formatted)
	}
}
