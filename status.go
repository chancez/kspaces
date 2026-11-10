package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// programRecord is one record of a window's program_status in `kitten @ ls`: what a program reported with
// the program status protocol (OSC 7501). kitty applies the protocol's lifetime rules, so a done or error
// record stays until the window is focused or typed in, and working and blocked ones end at the next prompt.
type programRecord struct {
	ID    string `json:"id"`
	State string `json:"state"`
	// What a blocked program waits for: permission, question or auth. Empty otherwise.
	Kind     string `json:"kind"`
	Progress *int   `json:"progress"`
	App      string `json:"app"`
	Title    string `json:"title"`
	Msg      string `json:"msg"`
	// When the record changed to its state, zero when not known. Not from kitty, see statusAtVar.
	Since time.Time `json:"-"`
}

// urgency orders states as kitty does when it picks the one record to show for several: higher first.
var urgency = map[string]int{"idle": 0, "working": 1, "done": 2, "error": 3, "blocked": 4}

// statusAtVar is the user variable kspaces_watcher.py in kitty.conf keeps on each window with the time
// each of its records last changed state, which kitty does not report.
const statusAtVar = "program_status_at"

// statusTimes reads statusAtVar: record id to the state it had and when it changed to it.
type statusTimes struct {
	Records map[string][2]any `json:"records"`
}

func (t statusTimes) since(r programRecord) time.Time {
	v, ok := t.Records[r.ID]
	if !ok {
		return time.Time{}
	}
	state, _ := v[0].(string)
	at, _ := v[1].(float64)
	if state != r.State || at == 0 {
		return time.Time{}
	}
	return time.Unix(0, int64(at*float64(time.Second)))
}

// Status is the most urgent program status of one window, and the window's other records.
type Status struct {
	Window  int
	Session string
	// The window's title, for telling apart several windows of one session.
	WindowTitle string
	programRecord
	// The window's other records, most urgent first, such as the other steps of a deploy reporting each
	// under its own id.
	Others []programRecord
}

// windowStatus picks the most urgent record of a window, or reports false when it has none. Idle records
// count: a program that reports idle is waiting for its next task.
func windowStatus(w lsWindow) (Status, bool) {
	var times statusTimes
	if raw := w.UserVars[statusAtVar]; raw != "" {
		// Times are a nicety: without them the entry just shows no age.
		_ = json.Unmarshal([]byte(raw), &times)
	}
	var records []programRecord
	for _, r := range w.ProgramStatus {
		if _, known := urgency[r.State]; known {
			r.Since = times.since(r)
			records = append(records, r)
		}
	}
	if len(records) == 0 {
		return Status{}, false
	}
	slices.SortStableFunc(records, func(a, b programRecord) int { return cmp.Compare(urgency[b.State], urgency[a.State]) })
	s := Status{Window: w.ID, Session: w.SessionName, WindowTitle: w.Title, programRecord: records[0]}
	if len(records) > 1 {
		s.Others = records[1:]
	}
	return s, true
}

// sortStatuses groups by state, most urgent first, and puts the most recent change first in each group.
func sortStatuses(s []Status) {
	slices.SortStableFunc(s, func(a, b Status) int {
		if c := cmp.Compare(urgency[b.State], urgency[a.State]); c != 0 {
			return c
		}
		if c := b.Since.Compare(a.Since); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Session, b.Session); c != 0 {
			return c
		}
		return cmp.Compare(a.Window, b.Window)
	})
}

// sessionStatus is the most urgent state among a session's windows, empty when none reports one other
// than idle.
func sessionStatus(statuses []Status, session string) string {
	ans := ""
	for _, s := range statuses {
		if s.Session == session && urgency[s.State] > urgency[ans] {
			ans = s.State
		}
	}
	return ans
}

// age is how long ago t was, in the largest whole unit: <1m, 5m, 3h, 2d. Empty for the zero time.
func age(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
}
