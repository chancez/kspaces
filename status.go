package main

import (
	"cmp"
	"slices"
)

// programRecord is one record of a window's program_status in `kitten @ ls`: what a program reported with
// the program status protocol (OSC 7501). kitty applies the protocol's lifetime rules, so a done or error
// record stays until the window is focused or typed in, and working and blocked ones end at the next prompt.
type programRecord struct {
	ID       string `json:"id"`
	State    string `json:"state"`
	Progress *int   `json:"progress"`
	App      string `json:"app"`
	Title    string `json:"title"`
	Msg      string `json:"msg"`
}

// urgency orders states as kitty does when it picks the one record to show for several: higher first.
var urgency = map[string]int{"idle": 0, "working": 1, "done": 2, "error": 3, "blocked": 4}

// Status is the most urgent program status of one window.
type Status struct {
	Window  int
	Session string
	// The window's title, for telling apart several windows of one session.
	WindowTitle string
	programRecord
}

// windowStatus picks the most urgent record of a window, or reports false when none is worth showing.
func windowStatus(w lsWindow) (Status, bool) {
	var best *programRecord
	for i := range w.ProgramStatus {
		r := &w.ProgramStatus[i]
		if best == nil || urgency[r.State] > urgency[best.State] {
			best = r
		}
	}
	if best == nil || urgency[best.State] == 0 {
		return Status{}, false
	}
	return Status{Window: w.ID, Session: w.SessionName, WindowTitle: w.Title, programRecord: *best}, true
}

// sortStatuses puts the most urgent first, then groups by session, so what needs attention is on top.
func sortStatuses(s []Status) {
	slices.SortStableFunc(s, func(a, b Status) int {
		if c := cmp.Compare(urgency[b.State], urgency[a.State]); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Session, b.Session); c != 0 {
			return c
		}
		return cmp.Compare(a.Window, b.Window)
	})
}

// sessionStatus is the most urgent state among a session's windows, empty when none reports one.
func sessionStatus(statuses []Status, session string) string {
	ans := ""
	for _, s := range statuses {
		if s.Session == session && urgency[s.State] > urgency[ans] {
			ans = s.State
		}
	}
	return ans
}
