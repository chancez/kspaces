package spaces

import (
	"slices"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/logs"
)

// Opening a copy afresh brings back the layout its host keeps, see Shared layouts in docs/kitty.md, so a
// copy left on one machine opens on another as it was: its tabs and splits, each pane attached to the
// session it showed.

// keptOpens are what opening the copy whose kitty session is session afresh opens of the layout its host
// keeps, by layoutHost: its panes whose sessions list still has running or shelved, in order, each a split
// of the pane it splits when that one opens too, else a tab. It is them as this machine's provider names
// them, too. Nothing without a layout kept, or one of its sessions left.
func keptOpens(session string, layoutHost Host, list *ProviderState) (sharedChange, []ProviderSession, error) {
	host, key := SplitCopy(session)
	got, err := layoutHost.LoadLayouts([]CopyLayout{{Key: key, Revision: -1}})
	if err != nil || len(got) == 0 || got[0].Layout == nil {
		return sharedChange{}, nil, err
	}
	live := map[string]ProviderSession{}
	if list != nil {
		for _, s := range list.Sessions {
			if s.State == "running" || s.State == "shelved" {
				live[s.ID] = s
			}
		}
	}
	all := mergeShared(nil, SharedLayout{}, *got[0].Layout)
	var c sharedChange
	var sessions []ProviderSession
	for _, add := range all.Open {
		s, ok := live[fromHost(host, add.Session)]
		if !ok {
			continue
		}
		if add.SplitOf != "" && !slices.ContainsFunc(c.Open, func(a sharedAdd) bool { return a.Session == add.SplitOf }) {
			add.SplitOf, add.Split, add.Size = "", "", 0
		}
		c.Open = append(c.Open, add)
		sessions = append(sessions, s)
	}
	return c, sessions, nil
}

// openKept opens wt's copy afresh with the layout its host keeps, see keptOpens, and says whether it did.
// Without a provider that lists and opens sessions, a host to ask, or a layout to open, it opens nothing.
func openKept(cfg config.Config, wt Worktree) bool {
	if len(cfg.Provider) == 0 {
		return false
	}
	caps := ProviderCapabilities(cfg.Provider)
	if !caps[CapList] || !caps[CapOpen] {
		return false
	}
	host, _ := SplitCopy(wt.Session)
	h, hangUp := ThisHost(), func() {}
	if host != "" {
		var err error
		if h, hangUp, err = HostOf(cfg, host); err != nil {
			return false
		}
	}
	defer hangUp()
	list, err := ListNow(cfg.Provider)
	if err != nil {
		logs.Error("listing the sessions to open "+wt.Session+" with", err)
		return false
	}
	change, sessions, err := keptOpens(wt.Session, h, list)
	if err != nil {
		logs.Error("loading the layout of "+wt.Session, err)
		return false
	}
	if change.Empty() {
		return false
	}
	ls := &layoutSync{open: openCommand, k: liveShared{}}
	if err := ls.apply(cfg, wt.Session, sharedCopy{host: host}, change, sessions, wt.Path, 0); err != nil {
		logs.Error("opening the layout of "+wt.Session, err)
	}
	return true
}
