package spaces

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/logs"
)

// A worktree's provider sessions are its panes: opening it shows every one, the saved file's and the kept
// layout's attaching again, and each other one opened in a tab of its own, as `kspaces adopt` does after
// OpenWorktree. An orphan, a session no window shows, is one of the worktree's when it belongs there, see
// homeOf: closing a pane with its session left running, or starting one outside kitty, as from another
// client, leaves one.

// adoptSettle is how long the orphans a worktree opening has must stay as they are before they are
// opened: a window the saved file or the kept layout opens shows its session only once its client
// attaches, and opening the session before would show it twice. adoptPoll is how often they are listed,
// and adoptMax how long they are waited for at most.
const (
	adoptPoll   = 750 * time.Millisecond
	adoptSettle = 2 * adoptPoll
	adoptMax    = 10 * time.Second
)

// homeOf is the worktree of space s belongs in: the one its worktree tag names, linked or missing, when
// space has it; else the deepest of space's worktrees holding its directory. ok is false for neither, as
// for a session from before tagging or from outside kspaces in no worktree's directory, which belongs in
// the main worktree. Another host's copy's worktrees are named after the host, see CopyName.
func homeOf(space Space, s ProviderSession) (Worktree, bool) {
	if dir := s.Tags["worktree"]; dir != "" {
		for _, wt := range slices.Concat(space.Worktrees, space.Missing) {
			if wt.Session == copyOf(wt.Host, space.Name+"@"+dir) {
				return wt, true
			}
		}
	}
	best := -1
	for i, wt := range space.Worktrees {
		if s.Cwd != "" && wt.Path != "" && (s.Cwd == wt.Path || strings.HasPrefix(s.Cwd, strings.TrimSuffix(wt.Path, "/")+"/")) &&
			(best < 0 || len(wt.Path) > len(space.Worktrees[best].Path)) {
			best = i
		}
	}
	if best < 0 {
		return Worktree{}, false
	}
	return space.Worktrees[best], true
}

// orphansOf are the sessions of sessions that are the orphans of space's worktree whose kitty session is
// session, see homeOf: shown by no window, kept for no kitty session, see keptHandles, and running or
// shelved, which opening brings back. default has none, as kitty's own session is no space's.
func orphansOf(space Space, session string, sessions []ProviderSession, kept map[string]bool) []ProviderSession {
	host, _ := SplitCopy(session)
	if space.Name == defaultSpace {
		return nil
	}
	var out []ProviderSession
	for _, s := range sessions {
		if s.Host != host || s.SpaceName() != space.Name || len(s.Windows) > 0 || kept[s.ID] || kept[s.Name] {
			continue
		}
		if s.State != "running" && s.State != "shelved" {
			continue
		}
		home := mainSession(space)
		if wt, ok := homeOf(space, s); ok {
			home = wt.Session
		}
		if home == session {
			out = append(out, s)
		}
	}
	return out
}

// localOrphans are the orphans of this machine's worktree whose kitty session is session, among sessions,
// none for another host's copy, whose sessions are its host's to end, see docs/stories.md.
func localOrphans(cfg config.Config, session string, sessions []ProviderSession) []ProviderSession {
	if host, _ := SplitCopy(session); host != "" {
		return nil
	}
	space, err := spaceOfSession(cfg, session)
	if err != nil {
		return nil
	}
	return orphansOf(space, session, sessions, keptHandles(cfg.SessionsDir))
}

// adopter opens a worktree's orphans, see AdoptOrphans, with its seams.
type adopter struct {
	cfg     config.Config
	space   Space
	session string
	now     func() time.Time
	sleep   func(time.Duration)
	list    func() (*ProviderState, error)
	kept    func() map[string]bool
	// anchor is a window of the session, for a tab to go beside, 0 for none.
	anchor func() int
	open   func(cfg config.Config, s ProviderSession) ([]string, string, error)
	launch func(session string, args []string) (int, error)
}

// run waits for the orphans to settle, then opens each in a tab of the session, by its anchor, with
// --keep-focus so the tab that has focus keeps it. One that fails is logged and the rest still go.
func (a adopter) run() error {
	start := a.now()
	var orphans []ProviderSession
	var last string
	var since time.Time
	for {
		a.sleep(adoptPoll)
		st, err := a.list()
		if err != nil {
			return err
		}
		orphans = orphansOf(a.space, a.session, st.Sessions, a.kept())
		var ids []string
		for _, s := range orphans {
			ids = append(ids, s.ID)
		}
		if key := strings.Join(ids, ","); key != last || since.IsZero() {
			last, since = key, a.now()
		}
		if a.now().Sub(since) >= adoptSettle || a.now().Sub(start) >= adoptMax {
			break
		}
	}
	host, _ := SplitCopy(a.session)
	home, _ := os.UserHomeDir()
	dir := home
	if host == "" && len(a.space.Worktrees) > 0 {
		dir = cmp.Or(a.space.Worktrees[0].Path, home)
		for _, wt := range a.space.Worktrees {
			if wt.Session == a.session {
				dir = cmp.Or(wt.Path, home)
			}
		}
	}
	for _, s := range orphans {
		argv, cwd, err := a.open(a.cfg, s)
		if err != nil {
			logs.Error("adopting "+s.Name, err)
			continue
		}
		// Another host's session starts here in the home directory, as its directory is the host's.
		at := dir
		if host == "" {
			at = cmp.Or(cwd, s.Cwd, dir)
		}
		var args []string
		if id := a.anchor(); id != 0 {
			args = append(args, fmt.Sprintf("--match=id:%d", id))
		}
		args = append(args, a.cfg.Launch...)
		args = append(args, "--type=tab", kitty.Quote("--cwd="+at))
		args = append(args, inSession(a.session)...)
		args = append(args, "--keep-focus")
		for _, w := range argv {
			args = append(args, kitty.Quote(w))
		}
		if _, err := a.launch(a.session, args); err != nil {
			logs.Error("adopting "+s.Name, err)
		}
	}
	return nil
}

// AdoptOrphans opens the orphans of the worktree whose kitty session is session, once they settle, each in
// a tab of its own there, see adoptSettle. A provider that does not list and open has none.
func AdoptOrphans(cfg config.Config, session string) error {
	caps := ProviderCapabilities(cfg.Provider)
	if !caps[CapList] || !caps[CapOpen] {
		return nil
	}
	space, err := spaceOfSession(cfg, session)
	if err != nil {
		return err
	}
	return adopter{
		cfg: cfg, space: space, session: session, now: time.Now, sleep: time.Sleep,
		list: func() (*ProviderState, error) { return ListNow(cfg.Provider) },
		kept: func() map[string]bool { return keptHandles(cfg.SessionsDir) },
		anchor: func() int {
			st, err := kitty.LoadState(kitty.FocusedWindow)
			if err != nil {
				return 0
			}
			return st.Sessions[session]
		},
		open: func(cfg config.Config, s ProviderSession) ([]string, string, error) {
			return openCommand(cfg.Provider, s)
		},
		launch: liveShared{}.Launch,
	}.run()
}

// adoptAfter has a background kspaces adopt the orphans of wt, opened now from no windows, see
// AdoptOrphans: the windows opening attach after this returns.
func adoptAfter(cfg config.Config, st kitty.State, wt Worktree) {
	if st.Windows[wt.Session] > 0 || len(cfg.Provider) == 0 {
		return
	}
	if err := kitty.LaunchBackground("adopt", "--", wt.Session); err != nil {
		logs.Error("adopting the sessions of "+wt.Session, err)
	}
}

// spaceOfSession is the space a worktree's kitty session is of, its worktrees listed: a configured space
// of this machine's, or of another host's for its copy, as its server has it, else a space of that one
// session, as a topic or ad hoc space is.
func spaceOfSession(cfg config.Config, session string) (Space, error) {
	host, key := SplitCopy(session)
	name, _, _ := strings.Cut(key, "@")
	if host == "" {
		for _, root := range cfg.Spaces {
			if s := LoadSpace(root); s.Name == name {
				return s, nil
			}
		}
		return Space{Name: name, Worktrees: []Worktree{{Session: session}}}, nil
	}
	h, hangUp, err := HostOf(cfg, host)
	if err != nil {
		return Space{}, err
	}
	defer hangUp()
	hd, err := h.Load(false)
	if err != nil {
		return Space{}, fmt.Errorf("asking %s: %w", host, err)
	}
	views, _ := RemoteViewsOf(map[string]RemoteHost{host: {Data: &hd}})
	for _, s := range views[host].Spaces {
		if s.Name == name {
			return s, nil
		}
	}
	return Space{Name: name, Worktrees: []Worktree{{Session: session, Host: host}}}, nil
}
