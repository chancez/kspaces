package spaces

import (
	"errors"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// recentSession is the open session other than st.Current that was focused most recently, false when there
// is none. Of equal times, as from a kitty that gives none, the first by name, so the choice is stable.
// Docks are no session's windows, so they neither make a session open nor count as focusing it.
func recentSession(st kitty.State) (string, bool) {
	best, bestAt := "", 0.0
	for session, at := range st.Focused {
		if session == st.Current {
			continue
		}
		if best == "" || at > bestAt || (at == bestAt && session < best) {
			best, bestAt = session, at
		}
	}
	return best, best != ""
}

// Recent switches to the session used before the focused window's, as alt-tab does, so twice goes back.
// Any open session counts, in a space or not. Only kitty and the config are read, as for Attend.
func Recent() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	st, err := kitty.LoadState(kitty.FocusedWindow)
	if err != nil {
		return err
	}
	session, ok := recentSession(st)
	if !ok {
		return errors.New("no other session has windows open")
	}
	// The session has windows, so opening it only switches to it: no path is needed.
	return OpenWorktree(cfg, st, Worktree{Session: session}, OpenOptions{})
}
