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
	// Ref is the space as asked for, HOST:NAME for another host's, see CopyName, and Name and Host it
	// apart, Host empty for this machine.
	Ref  string
	Name string
	Host string
	// AdHoc is an ad hoc space, its host's server's, see AdHocSpace.
	AdHoc bool
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

// PlanSpaceRemoval is what removing the space ref would do, as kitty, the config, the provider and the
// space's host have it now. A space neither configured, ad hoc nor the provider's, and default, which every
// provider keeps, are refused. HOST:NAME is another host's space of its sessions in no space, see
// CopyName, which its host removes, ending its sessions there.
func PlanSpaceRemoval(cfg config.Config, ref string) (SpaceRemoval, error) {
	r := SpaceRemoval{Ref: ref, Name: ref}
	if host, name := SplitCopy(ref); host != "" {
		if _, ok := cfg.Hosts[host]; ok {
			r.Host, r.Name = host, name
		}
	}
	name := r.Name
	if r.Host == "" {
		for _, root := range cfg.Spaces {
			if LoadSpace(root).Name == name {
				r.Root = root
			}
		}
	}
	if r.Root == "" {
		r.AdHoc = hostHasAdHoc(cfg, r.Host, name)
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
	provided := st != nil && (slices.Contains(st.Spaces, name) && r.Host == "" ||
		slices.ContainsFunc(st.Sessions, func(s ProviderSession) bool { return s.Host == r.Host && s.Host != "" && s.SpaceName() == name }))
	if r.Root == "" && (name == defaultSpace || !r.AdHoc && !provided) {
		return r, fmt.Errorf("%s is not a space kspaces can remove", ref)
	}
	state := kitty.StateOf(data, kitty.FocusedWindow)
	saved := savedSessions(cfg.SessionsDir)
	mine := func(s string) bool { return s == name || strings.HasPrefix(s, name+"@") }
	if r.Root == "" {
		mine = func(s string) bool { return s == ref }
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
			if p.SpaceName() == name && p.Host == r.Host || shown[p.ID] {
				r.Provider = append(r.Provider, p)
			}
		}
	}
	switch {
	case r.Root != "":
		r.Fates = Fates(r.Caps, false)
	case r.Host != "" || r.AdHoc && len(r.Provider) == 0:
		// Its host ends its sessions there, or it has none to end.
		r.Fates = []Fate{FateEnd}
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
func RemoveSpace(cfg config.Config, ref string, fate Fate) error {
	r, err := PlanSpaceRemoval(cfg, ref)
	if err != nil {
		return err
	}
	name := r.Name
	if r.Root == "" && fate != FateEnd {
		return fmt.Errorf("%s holds the provider's sessions, which have to end for it to go", name)
	}
	var errs []error
	if fate != FateLeave && r.Host == "" {
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
	if r.Host != "" {
		// Its host ends its sessions there, removes the provider's space, and forgets it.
		h, hangUp, err := HostOf(cfg, r.Host)
		if err != nil {
			return errors.Join(append(errs, err)...)
		}
		defer hangUp()
		return errors.Join(append(errs, h.RemoveSpace(name))...)
	}
	if r.Root != "" {
		errs = append(errs, config.SaveRemoveSpace(r.Root))
	}
	if r.Caps[CapRemoveSpace] && (r.Root == "" || fate == FateEnd) && !r.AdHoc {
		if err := RemoveProviderSpace(cfg.Provider, name); err != nil && r.Root == "" {
			errs = append(errs, err)
		}
	}
	if r.AdHoc {
		// Its server removes the provider's space, its sessions ended above, and forgets it.
		errs = append(errs, ThisHost().RemoveSpace(name))
	}
	return errors.Join(errs...)
}

// AskRemoveSpace opens the form that confirms removing the space name, in a popup sized as
// AskCloseSession's.
func AskRemoveSpace(cfg config.Config, st kitty.State, name string) error {
	return launchForm(cfg, st, formPanel, "remove-space", name)
}

// RemoveSpaceInBackground runs `kspaces remove-space --yes` for the space ref, see PlanSpaceRemoval, as a
// kitty background process: removing closes the space's windows, and the form that asked may be over one
// of them.
func RemoveSpaceInBackground(name string, fate Fate) error {
	args := []string{"remove-space", "--yes"}
	if fate != "" {
		args = append(args, "--sessions", string(fate))
	}
	return kitty.LaunchBackground(append(args, "--", name)...)
}

// hostHasAdHoc says whether the space name is an ad hoc space of host, "" for this machine, as its server
// says.
func hostHasAdHoc(cfg config.Config, host, name string) bool {
	h, hangUp := ThisHost(), func() {}
	if host != "" {
		var err error
		if h, hangUp, err = HostOf(cfg, host); err != nil {
			return false
		}
	}
	defer hangUp()
	hd, err := h.Load(false)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(hd.AdHoc, func(a AdHocSpace) bool { return a.Name == name })
}
