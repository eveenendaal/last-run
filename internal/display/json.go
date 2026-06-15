package display

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/eveenendaal/last-run/internal/db"
	"github.com/eveenendaal/last-run/internal/format"
)

type taskJSON struct {
	ID                        string  `json:"id"`
	LastRun                   *string `json:"last_run"`
	TimeSinceLastRun          *int64  `json:"time_since_last_run"`
	TimeSinceLastRunFormatted *string `json:"time_since_last_run_formatted"`
	StartTime                 *string `json:"start_time"`
	ElapsedTime               *int64  `json:"elapsed_time"`
	ElapsedTimeFormatted      *string `json:"elapsed_time_formatted"`
	Duration                  *int64  `json:"duration"`
	DurationFormatted         *string `json:"duration_formatted"`
	Status                    string  `json:"status"`
}

type statusJSON struct {
	Tasks     []taskJSON `json:"tasks"`
	Timestamp string     `json:"timestamp"`
}

// PrintTaskStatusJSON serializes task status as pretty-printed JSON, matching
// the shape and field order of the original Rust output.
func PrintTaskStatusJSON(tasks []db.TaskStatus) {
	now := time.Now().UTC()

	jsonTasks := make([]taskJSON, 0, len(tasks))
	for _, t := range tasks {
		var timeSince *int64
		var timeSinceFmt *string
		if t.LastRun != nil {
			ms := now.Sub(*t.LastRun).Milliseconds()
			timeSince = &ms
			f := format.FormatDuration(now.Sub(*t.LastRun))
			timeSinceFmt = &f
		}

		var elapsed *int64
		switch {
		case t.StartTime != nil && t.LastRun != nil && t.StartTime.Before(*t.LastRun):
			ms := t.LastRun.Sub(*t.StartTime).Milliseconds()
			elapsed = &ms
		case t.StartTime != nil && t.LastRun == nil:
			ms := now.Sub(*t.StartTime).Milliseconds()
			elapsed = &ms
		}

		var elapsedFmt *string
		if elapsed != nil {
			f := format.FormatDuration(time.Duration(*elapsed) * time.Millisecond)
			elapsedFmt = &f
		}

		status := statusString(t, now)

		var lastRunStr, startTimeStr, durationFmt *string
		if t.LastRun != nil {
			s := format.FormatRFC3339(*t.LastRun)
			lastRunStr = &s
		}
		if t.StartTime != nil {
			s := format.FormatRFC3339(*t.StartTime)
			startTimeStr = &s
		}
		if t.Duration != nil {
			s := format.FormatDuration(time.Duration(*t.Duration) * time.Second)
			durationFmt = &s
		}

		jsonTasks = append(jsonTasks, taskJSON{
			ID:                        t.ID,
			LastRun:                   lastRunStr,
			TimeSinceLastRun:          timeSince,
			TimeSinceLastRunFormatted: timeSinceFmt,
			StartTime:                 startTimeStr,
			ElapsedTime:               elapsed,
			ElapsedTimeFormatted:      elapsedFmt,
			Duration:                  t.Duration,
			DurationFormatted:         durationFmt,
			Status:                    status,
		})
	}

	output := statusJSON{Tasks: jsonTasks, Timestamp: format.FormatRFC3339(now)}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&output); err != nil {
		return
	}
	// Encoder.Encode appends a trailing newline; Println adds its own, so trim.
	fmt.Print(buf.String())
}

// statusString computes a task's status string, matching the TUI/JSON logic.
func statusString(t db.TaskStatus, now time.Time) string {
	if t.StartTime != nil && t.LastRun == nil {
		return "running"
	}
	if t.LastRun != nil {
		if t.Duration != nil {
			if now.Sub(*t.LastRun) > time.Duration(*t.Duration)*time.Second {
				return "due"
			}
			return "ok"
		}
		return "ok"
	}
	return "unknown"
}
