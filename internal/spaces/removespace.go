package spaces

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// Removing a space ends what kspaces holds of it: the kitty sessions of its worktrees, or of a topic
// space its one session, closed and their saved files deleted, as nothing lists them under a space any
// more; the provider's sessions in it done a fate to, see Fate; and the space itself, from the config for
// a configured one, which leaves its repository alone, and from the provider for a topic space.

// CapRemoveSpace is the provider removing one of its spaces, remove-space.
const CapRemoveSpace = "remove-space"

// RemoveProviderSpace has provider remove its space name, which it refuses as it must, as one still
// holding sessions.
func RemoveProviderSpace(provider []string, name string) error {
	return askProvider(provider, "remove-space", struct {
		Space string `json:"space"`
	}{name}, nil)
}

// SpaceRemoval is what removing a space would do, for the form that confirms it.
type SpaceRemoval struct {
	Name string
	// Root is a configured space's, empty for a topic space.
	Root string
	// Sessions are its kitty sessions with windows or a saved file, which go.
	Sessions []string
	// Provider are the provider's sessions in it, and those its kitty sessions' windows show.
	Provider []ProviderSession
	Caps     Capabilities
	// Fates are what can become of Provider, FateEnd first: a topic space's is FateEnd alone, as the
	// provider removes only an empty space, and none when the provider cannot remove it.
	Fates []Fate
}

// PlanSpaceRemoval is what removing the space name would do, as kitty, the config and the provider have it
// now. A space neither configured nor the provider's, and default, which every provider keeps, are
// refused.
func PlanSpaceRemoval(cfg config.Config, name string) (SpaceRemoval, error) {
	r := SpaceRemoval{Name: name}
	for _, root := range cfg.Spaces {
		if LoadSpace(root).Name == name {
			r.Root = root
		}
	}
	if len(cfg.Provider) > 0 {
		r.Caps = ProviderCapabilities(cfg.Provider)
	}
	data, err := kitty.ListWindows()
	if err != nil {
		return r, err
	}
	var st *ProviderState
	if r.Caps[CapList] {
		if st, err = listSessions(cfg.Provider, allWindows(data)); err != nil {
			return r, err
		}
	}
	if r.Root == "" && (name == defaultSpace || st == nil || !slices.Contains(st.Spaces, name)) {
		return r, fmt.Errorf("%s is not a space kspaces can remove", name)
	}
	state := kitty.StateOf(data, kitty.FocusedWindow)
	saved := savedSessions(cfg.SessionsDir)
	mine := func(s string) bool { return s == name || strings.HasPrefix(s, name+"@") }
	if r.Root == "" {
		mine = func(s string) bool { return s == name }
	}
	for s := range state.Windows {
		if mine(s) {
			r.Sessions = append(r.Sessions, s)
		}
	}
	for _, s := range saved {
		if mine(s) && !slices.Contains(r.Sessions, s) {
			r.Sessions = append(r.Sessions, s)
		}
	}
	slices.Sort(r.Sessions)
	if st != nil {
		shown := map[string]bool{}
		for _, s := range r.Sessions {
			for _, p := range ClosingSessions(st, data, s, false) {
				shown[p.ID] = true
			}
		}
		for _, p := range st.Sessions {
			if p.SpaceName() == name || shown[p.ID] {
				r.Provider = append(r.Provider, p)
			}
		}
	}
	switch {
	case r.Root != "":
		r.Fates = Fates(r.Caps, false)
	case r.Caps[CapRemoveSpace] && (len(r.Provider) == 0 || r.Caps[CapKill] || r.Caps[CapRemove]):
		r.Fates = []Fate{FateEnd}
	}
	return r, nil
}

// RemoveSpace removes the space name, doing fate to the provider's sessions in it, see PlanSpaceRemoval:
// the sessions first, then its kitty sessions, closed and forgotten, then the space, from the config, and
// from the provider when it was ended, a topic space always. Leaving the sessions running marks the
// windows kept, which tells a close watcher to leave them alone, and keeps the provider's space, which
// then lists as a topic space holding them. One failing does not stop the rest, which were confirmed.
func RemoveSpace(cfg config.Config, name string, fate Fate) error {
	r, err := PlanSpaceRemoval(cfg, name)
	if err != nil {
		return err
	}
	if r.Root == "" && fate != FateEnd {
		return fmt.Errorf("%s holds the provider's sessions, which have to end for it to go", name)
	}
	var errs []error
	if fate != FateLeave {
		errs = append(errs, ApplyFate(cfg.Provider, r.Caps, fate, r.Provider))
	}
	state, err := kitty.LoadState(kitty.FocusedWindow)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	for _, s := range r.Sessions {
		if state.Windows[s] > 0 {
			if fate == FateLeave {
				errs = append(errs, kitty.SetSessionVar(s, kitty.KeepVar, string(KeepClosed)))
			}
			if err := CloseSession(s); err != nil {
				errs = append(errs, err)
				continue
			}
		}
		errs = append(errs, endFile(cfg, s))
	}
	if r.Root != "" {
		errs = append(errs, config.SaveRemoveSpace(r.Root))
	}
	if r.Caps[CapRemoveSpace] && (r.Root == "" || fate == FateEnd) {
		if err := RemoveProviderSpace(cfg.Provider, name); err != nil && r.Root == "" {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// AskRemoveSpace opens the form that confirms removing the space name, in a popup sized as
// AskCloseSession's.
func AskRemoveSpace(cfg config.Config, st kitty.State, name string) error {
	return launchForm(cfg, st, formPanel, "remove-space", name)
}

// RemoveSpaceInBackground runs `kspaces remove-space --yes` as a kitty background process: removing
// closes the space's windows, and the form that asked may be over one of them.
func RemoveSpaceInBackground(name string, fate Fate) error {
	args := []string{"remove-space", "--yes"}
	if fate != "" {
		args = append(args, "--sessions", string(fate))
	}
	return kitty.LaunchBackground(append(args, "--", name)...)
}
