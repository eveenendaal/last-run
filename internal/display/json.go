package display

import (
	"encoding/json"
	"io"
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

// WriteTaskStatusJSON writes the task status snapshot for `status --json` as
// pretty-printed JSON. Durations are reported in milliseconds (elapsed, time
// since last run) or seconds (check duration), each with a formatted twin.
func WriteTaskStatusJSON(w io.Writer, tasks []db.TaskStatus, now time.Time) error {
	jsonTasks := make([]taskJSON, 0, len(tasks))
	for _, t := range tasks {
		jt := taskJSON{ID: t.ID, Duration: t.Duration, Status: t.Status(now)}
		if t.LastRun != nil {
			jt.LastRun = new(format.FormatRFC3339(*t.LastRun))
			jt.TimeSinceLastRun, jt.TimeSinceLastRunFormatted = durationFields(now.Sub(*t.LastRun))
		}
		if t.StartTime != nil {
			jt.StartTime = new(format.FormatRFC3339(*t.StartTime))
		}
		if elapsed, ok := t.Elapsed(now); ok {
			jt.ElapsedTime, jt.ElapsedTimeFormatted = durationFields(elapsed)
		}
		if t.Duration != nil {
			jt.DurationFormatted = new(format.FormatDuration(time.Duration(*t.Duration) * time.Second))
		}
		jsonTasks = append(jsonTasks, jt)
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(statusJSON{Tasks: jsonTasks, Timestamp: format.FormatRFC3339(now)})
}

// durationFields returns d in milliseconds alongside its formatted string.
func durationFields(d time.Duration) (*int64, *string) {
	return new(d.Milliseconds()), new(format.FormatDuration(d))
}
