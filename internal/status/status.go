// Package status reads what programs report with the program status protocol (OSC 7501), as kitty keeps
// it for each window.
package status

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// Record is one record of a window's program_status in `kitten @ ls`: what a program reported with the
// program status protocol (OSC 7501). kitty applies the protocol's lifetime rules, so a done or error
// record stays until the window is focused or typed in, and working and blocked ones end at the next prompt.
type Record struct {
	ID    string `json:"id"`
	State string `json:"state"`
	// What a blocked program waits for: permission, question or auth. Empty otherwise.
	Kind     string `json:"kind"`
	Progress *int   `json:"progress"`
	App      string `json:"app"`
	Title    string `json:"title"`
	Msg      string `json:"msg"`
	// When the record changed to its state, zero when not known. Not from kitty, see AtVar.
	Since time.Time `json:"-"`
}

// urgency orders states as kitty does when it picks the one record to show for several: higher first.
var urgency = map[string]int{"idle": 0, "working": 1, "done": 2, "error": 3, "blocked": 4}

// Urgency is how urgent a state is, higher first; 0 for idle and for no state.
func Urgency(state string) int {
	return urgency[state]
}

// AtVar is the user variable kspaces_watcher.py in kitty.conf keeps on each window with the time each of
// its records last changed state, which kitty does not report.
const AtVar = "program_status_at"

// times reads AtVar: record id to the state it had and when it changed to it.
type times struct {
	Records map[string][2]any `json:"records"`
}

func (t times) since(r Record) time.Time {
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
	Record
	// The window's other records, most urgent first, such as the other steps of a deploy reporting each
	// under its own id.
	Others []Record
}

// OfWindow picks the most urgent record of a window, or reports false when it has none. Idle records
// count: a program that reports idle is waiting for its next task. vars are the window's user variables,
// where AtVar is.
func OfWindow(id int, session, title string, vars map[string]string, records []Record) (Status, bool) {
	var at times
	if raw := vars[AtVar]; raw != "" {
		// Times are a nicety: without them the entry just shows no age.
		_ = json.Unmarshal([]byte(raw), &at)
	}
	var known []Record
	for _, r := range records {
		if _, ok := urgency[r.State]; ok {
			r.Since = at.since(r)
			known = append(known, r)
		}
	}
	if len(known) == 0 {
		return Status{}, false
	}
	slices.SortStableFunc(known, func(a, b Record) int { return cmp.Compare(urgency[b.State], urgency[a.State]) })
	s := Status{Window: id, Session: session, WindowTitle: title, Record: known[0]}
	if len(known) > 1 {
		s.Others = known[1:]
	}
	return s, true
}

// Sort groups by state, most urgent first, and puts the most recent change first in each group.
func Sort(s []Status) {
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

// OfSession is the most urgent state among a session's windows, empty when none reports one other than
// idle.
func OfSession(statuses []Status, session string) string {
	ans := ""
	for _, s := range statuses {
		if s.Session == session && urgency[s.State] > urgency[ans] {
			ans = s.State
		}
	}
	return ans
}

// Age is how long ago t was, in the largest whole unit: <1m, 5m, 3h, 2d. Empty for the zero time.
func Age(t, now time.Time) string {
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
