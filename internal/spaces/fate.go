package spaces

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/chancez/kspaces/internal/kitty"
)

// Closing a session's windows, or removing a worktree, ends the windows; what runs in them is the session
// provider's, and what becomes of it is chosen in the form that confirms, as a Fate. Left to itself, a
// multiplexer's close watcher decides instead: cm's shelves an idle session and detaches a busy one, so a
// removed worktree's sessions came back as detached rows of its space.

// Fate is what becomes of the provider's sessions the closing windows show.
type Fate string

const (
	// FateEnd kills the running sessions and removes the ended ones, the default: closing or removing is
	// asked to end the work.
	FateEnd Fate = "end"
	// FateShelve keeps them to come back as they were: for a closed session the provider's keep, with the
	// session's file, else its shelve.
	FateShelve Fate = "shelve"
	// FateLeave leaves them as they are, running, the windows alone closing.
	FateLeave Fate = "leave"
)

// ParseFate is the fate named s, FateEnd for empty.
func ParseFate(s string) (Fate, error) {
	switch f := Fate(s); f {
	case "":
		return FateEnd, nil
	case FateEnd, FateShelve, FateLeave:
		return f, nil
	}
	return "", fmt.Errorf("%q is not end, shelve or leave", s)
}

// Fates are the fates caps allows for sessions closing, FateEnd first, nil when there is no choice to make:
// without a provider, or with one that can only leave them. keeps is the session being kept to open again,
// whose Shelve is the provider's keep rather than its shelve.
func Fates(caps Capabilities, keeps bool) []Fate {
	var out []Fate
	if caps[CapList] && (caps[CapKill] || caps[CapRemove]) {
		out = append(out, FateEnd)
	}
	if keeps && caps[CapKeep] || caps[CapList] && caps[CapShelve] {
		out = append(out, FateShelve)
	}
	if len(out) == 0 {
		return nil
	}
	return append(out, FateLeave)
}

// windowsOf are the ids of session's windows in data, kspaces' own left out.
func windowsOf(data []kitty.OSWindow, session string) []int {
	var ids []int
	for _, osw := range data {
		for _, tab := range osw.Tabs {
			for _, w := range tab.Windows {
				if w.SessionName == session && !kitty.Own(w) {
					ids = append(ids, w.ID)
				}
			}
		}
	}
	return ids
}

// ClosingSessions are the provider's sessions in st that the kitty session's windows in data show, and,
// for a linked worktree's session SPACE@DIR, those of SPACE whose worktree tag is DIR, whose directory is
// about to go with it. In st's order, each once.
func ClosingSessions(st *ProviderState, data []kitty.OSWindow, session string, worktree bool) []ProviderSession {
	if st == nil {
		return nil
	}
	ids := windowsOf(data, session)
	space, dir, linked := strings.Cut(session, "@")
	var out []ProviderSession
	for _, s := range st.Sessions {
		shown := slices.ContainsFunc(s.Windows, func(id int) bool { return slices.Contains(ids, id) })
		tagged := worktree && linked && dir != "" && s.SpaceName() == space && s.Tags["worktree"] == dir
		if shown || tagged {
			out = append(out, s)
		}
	}
	return out
}

// ApplyFate does fate to sessions with provider, as caps allows: FateEnd kills the running ones and removes
// the ended ones, FateShelve shelves the running ones, and FateLeave does nothing. A session the provider
// has no verb for is left as it is. One failing does not stop the rest, and says which it was.
func ApplyFate(provider []string, caps Capabilities, fate Fate, sessions []ProviderSession) error {
	var errs []error
	for _, s := range sessions {
		var err error
		switch {
		case fate == FateEnd && !s.Ended() && caps[CapKill]:
			err = KillSession(provider, s)
		case fate == FateEnd && s.Ended() && caps[CapRemove]:
			err = RemoveSession(provider, s)
		case fate == FateShelve && !s.Ended() && caps[CapShelve]:
			err = ShelveSession(provider, s)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.Name, err))
		}
	}
	return errors.Join(errs...)
}

// FateWords says what fate does to sessions, for the forms that confirm it: how many, and what the busy
// ones run, as ending or shelving one stops it. Empty for none.
func FateWords(fate Fate, sessions []ProviderSession) string {
	if len(sessions) == 0 {
		return ""
	}
	n := fmt.Sprintf("%d sessions", len(sessions))
	if len(sessions) == 1 {
		n = "1 session"
	}
	var says string
	switch fate {
	case FateEnd:
		says = "Ends " + n
	case FateShelve:
		says = "Shelves " + n + ", to bring back later"
	default:
		return "Leaves " + n + " running"
	}
	var busy []string
	for _, s := range sessions {
		if s.Busy && !s.Ended() {
			what := s.Command
			if what == "" {
				what = "a command"
			}
			busy = append(busy, s.Name+" running "+what)
		}
	}
	if len(busy) > 0 {
		says += ": " + strings.Join(busy, ", ")
	}
	return says
}

// ApplyClosingFate does fate to the provider's sessions the kitty session's windows show, see
// ClosingSessions, listing them now, before the windows close: a session no window shows any more is one
// the provider cannot match. With worktree, those of the space tagged with the session's worktree too.
func ApplyClosingFate(provider []string, session string, fate Fate, worktree bool) error {
	if fate == FateLeave || len(provider) == 0 {
		return nil
	}
	caps := ProviderCapabilities(provider)
	sessions, err := closingSessionsNow(provider, caps, session, worktree)
	if err != nil {
		return err
	}
	return ApplyFate(provider, caps, fate, sessions)
}

// closingSessionsNow are ClosingSessions for session as kitty and the provider list them now, none from a
// provider that does not list. Asked before anything marks or closes the windows: a window marked kept is
// left out of what the provider is told, as one of kspaces' own, and one whose session ended is gone.
func closingSessionsNow(provider []string, caps Capabilities, session string, worktree bool) ([]ProviderSession, error) {
	if !caps[CapList] {
		return nil, nil
	}
	data, err := kitty.ListWindows()
	if err != nil {
		return nil, err
	}
	st, err := listSessions(provider, allWindows(data))
	if err != nil {
		return nil, err
	}
	return ClosingSessions(st, data, session, worktree), nil
}

// Closing is what closing sessions would do to the provider's sessions, for a form that confirms it: what
// the provider does, the fates it allows, and the sessions each kitty session's windows show. Without a
// provider that lists, there are none, and keeps says the sessions are kept to open again, see Fates.
type Closing struct {
	Caps     Capabilities
	Fates    []Fate
	Sessions map[string][]ProviderSession
}

// ClosingNow is Closing for sessions, kitty's windows as they are, with worktree the space's sessions
// tagged with each session's worktree too. A provider that cannot list now offers no choice but what it
// keeps.
func ClosingNow(provider []string, sessions []string, keeps, worktree bool) Closing {
	if len(provider) == 0 {
		return Closing{}
	}
	caps := ProviderCapabilities(provider)
	c := Closing{Caps: caps, Sessions: map[string][]ProviderSession{}}
	if caps[CapList] {
		data, err := kitty.ListWindows()
		var st *ProviderState
		if err == nil {
			st, err = listSessions(provider, allWindows(data))
		}
		if err != nil {
			caps = Capabilities{CapKeep: caps[CapKeep]}
		}
		for _, s := range sessions {
			c.Sessions[s] = ClosingSessions(st, data, s, worktree)
		}
	}
	c.Fates = Fates(caps, keeps)
	return c
}
