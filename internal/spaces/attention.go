package spaces

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/status"
)

// attentionStates are the states of a program waiting for me: asking, failed or finished. Working and idle
// ones are not.
var attentionStates = map[string]bool{"blocked": true, "error": true, "done": true}

// Attention are the windows whose program needs me, most urgent first, as the status list sorts them:
// blocked, then failed, then done, the most recent change first within a state.
func Attention(statuses []status.Status) []status.Status {
	var ans []status.Status
	for _, s := range statuses {
		if attentionStates[s.State] {
			ans = append(ans, s)
		}
	}
	status.Sort(ans)
	return ans
}

// Shown are the statuses but those of the sessions of spaces in hidden groups, which are out of sight: a
// space's sessions by their names, the space's own or SPACE@DIR, as only the config is read. A session in
// no space is always shown.
func Shown(cfg config.Config, hidden map[string]bool, statuses []status.Status) []status.Status {
	var gone []string
	for _, root := range cfg.Spaces {
		if name := filepath.Base(root); hidden[cfg.Space[name].Group] {
			gone = append(gone, name)
		}
	}
	if len(gone) == 0 {
		return statuses
	}
	var ans []status.Status
	for _, s := range statuses {
		if !slices.ContainsFunc(gone, func(name string) bool { return s.Session == name || strings.HasPrefix(s.Session, name+"@") }) {
			ans = append(ans, s)
		}
	}
	return ans
}

// attentionStep is the status delta places from the window current among those that need me, wrapping
// around, or false when none does. From a window that is not one of them, a step forward lands on the most
// urgent and a step back on the least.
func attentionStep(statuses []status.Status, current, delta int) (status.Status, bool) {
	need := Attention(statuses)
	if len(need) == 0 {
		return status.Status{}, false
	}
	at := -1
	for i, s := range need {
		if s.Window == current {
			at = i
		}
	}
	return need[cycleIndex(len(need), at, delta)], true
}

// Attend focuses the window delta places from the focused one among those that need me, see
// attentionStep, but for those in hidden groups, see Shown. With none, it does nothing: there is nothing
// to go to, which is not an error. Only kitty, the config and the hidden groups are read, as the spaces'
// worktrees play no part.
func Attend(delta int) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	st, err := kitty.LoadState(kitty.FocusedWindow)
	if err != nil {
		return err
	}
	// Seen from the focused window, Home is that window: the one focused most recently in its tab.
	s, ok := attentionStep(Shown(cfg, LoadHidden(), st.Statuses), st.Home, delta)
	if !ok {
		return nil
	}
	return OpenWindow(cfg, st, s.Window, s.Session)
}
