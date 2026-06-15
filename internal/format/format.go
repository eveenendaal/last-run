// Package format provides duration parsing/formatting and timestamp helpers
// that mirror the original Rust implementation's behavior exactly.
package format

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// rfc3339Layout renders timestamps in UTC with a "+00:00" offset and
// trailing-zero-trimmed fractional seconds, closely matching chrono's
// to_rfc3339() output so existing databases stay byte-compatible.
const rfc3339Layout = "2006-01-02T15:04:05.999999999-07:00"

// ParseDuration parses a human-readable duration string into a time.Duration.
//
// Accepted suffixes: h (hours), d (days), w (weeks), m (months = 30 days).
// A bare number, an unknown suffix, or an unparseable value is an error.
func ParseDuration(durationStr string) (time.Duration, error) {
	if s, ok := strings.CutSuffix(durationStr, "h"); ok {
		if hours, err := strconv.ParseInt(s, 10, 64); err == nil {
			return time.Duration(hours) * time.Hour, nil
		}
	} else if s, ok := strings.CutSuffix(durationStr, "d"); ok {
		if days, err := strconv.ParseInt(s, 10, 64); err == nil {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	} else if s, ok := strings.CutSuffix(durationStr, "w"); ok {
		if weeks, err := strconv.ParseInt(s, 10, 64); err == nil {
			return time.Duration(weeks) * 7 * 24 * time.Hour, nil
		}
	} else if s, ok := strings.CutSuffix(durationStr, "m"); ok {
		if months, err := strconv.ParseInt(s, 10, 64); err == nil {
			return time.Duration(months*30) * 24 * time.Hour, nil
		}
	}

	return 0, fmt.Errorf(
		"Invalid duration format: '%s'. Use a number followed by h (hours), d (days), w (weeks), or m (months). Examples: 24h, 7d, 2w, 3m",
		durationStr,
	)
}

// FormatDuration renders a duration as a compact human-readable string.
//
//	< 1m      -> "S.HHs"  (seconds to hundredths)
//	< 1h      -> "MmSs"
//	>= 1d     -> "DdHh"   (minutes omitted)
//	otherwise -> "HhMm"
func FormatDuration(d time.Duration) string {
	ms := d.Milliseconds()
	sign := ""
	if ms < 0 {
		sign = "-"
	}
	absMs := ms
	if absMs < 0 {
		absMs = -absMs
	}

	totalSeconds := absMs / 1000

	if totalSeconds < 60 {
		seconds := absMs / 1000
		hundredths := (absMs % 1000) / 10
		return fmt.Sprintf("%s%d.%02ds", sign, seconds, hundredths)
	} else if totalSeconds < 3600 {
		minutes := totalSeconds / 60
		seconds := totalSeconds % 60
		return fmt.Sprintf("%s%dm%ds", sign, minutes, seconds)
	}

	totalMinutes := totalSeconds / 60
	days := totalMinutes / (24 * 60)
	hours := (totalMinutes % (24 * 60)) / 60
	minutes := totalMinutes % 60

	if days > 0 {
		return fmt.Sprintf("%s%dd%dh", sign, days, hours)
	}
	return fmt.Sprintf("%s%dh%dm", sign, hours, minutes)
}

// FormatRFC3339 formats a time as an RFC3339 string in UTC, matching the
// format used when persisting timestamps to the database.
func FormatRFC3339(t time.Time) string {
	return t.UTC().Format(rfc3339Layout)
}

// ParseRFC3339Opt parses an optional RFC3339 string into a *time.Time,
// silently discarding parse errors (returns nil).
func ParseRFC3339Opt(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

// FormatDatetime formats a UTC time as "YYYY-MM-DD HH:MM:SS" in the local
// timezone.
func FormatDatetime(t time.Time) string {
	return t.Local().Format("2006-01-02 15:04:05")
}
