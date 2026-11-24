package spaces

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/logs"
)

// Keeping a session closes its windows so that opening it again brings them back. Its saved file keeps its
// tabs and splits, which goto_session opens. What runs in its windows is the session provider's to keep, a
// program the config names, see provider.go: one running each window in a multiplexer session keeps that
// session, and the window attaching to it again when the file reopens it brings it back. Without a
// provider, or one that does not keep, the layout alone is kept.

// KeepMode is how KeepSession keeps a session.
type KeepMode string

const (
	// KeepClosed keeps the session to open again: its file, and what the provider keeps for a while, as the
	// provider decides. What closing a session does.
	KeepClosed KeepMode = "close"
	// KeepSuspended keeps the session as KeepClosed does, and what the provider keeps until the session is
	// opened again or forgotten.
	KeepSuspended KeepMode = "suspend"
	// KeepClosing and KeepSuspending are KeepClosed and KeepSuspended while the provider keeps: listed with
	// the provider's sessions the windows show, before it is asked, so no list taken while it shelves them
	// sees them as detached, and replaced with the mode once it says what it kept. See keepSession.
	KeepClosing    KeepMode = "closing"
	KeepSuspending KeepMode = "suspending"
)

// keeping is the mode listed while the provider keeps a session as m.
func (m KeepMode) keeping() KeepMode {
	if m == KeepSuspended {
		return KeepSuspending
	}
	return KeepClosing
}

// Settled is the mode a session listed as m is kept as once the provider has kept it, m itself when it is
// not KeepClosing or KeepSuspending.
func (m KeepMode) Settled() KeepMode {
	switch m {
	case KeepSuspending:
		return KeepSuspended
	case KeepClosing:
		return KeepClosed
	}
	return m
}

// Keeping says the provider is keeping a session listed as m, see KeepClosing.
func (m KeepMode) Keeping() bool { return m.Settled() != m }

// keptFile, in the sessions directory, lists the kept sessions, a line each of tab-separated fields: the
// mode, the session, then what the provider kept of it. A mode the provider is still keeping as, see
// KeepClosing, is followed by a colon and when it was listed, in Unix seconds. save_sessions.py reads it too: it keeps the file of a
// session listed there when its last window closes, and drops the session from the list once it has
// windows again.
const keptFile = ".kept-sessions"

// Kept is a session KeepSession kept.
type Kept struct {
	Mode KeepMode
	// What the provider said it kept, which it is given back to forget, see provider.go. While Mode is
	// Keeping, the ids and names of the provider's sessions the windows showed.
	Handles []string
	// Since is when a Keeping mode was listed, zero for any other.
	Since time.Time
}

// CanKeep says whether sessions can be kept: their files are written by the session saver.
func CanKeep(cfg config.Config) bool {
	return cfg.SessionsDir != "" && cfg.Kitty.SaveSessions
}

// LoadKept is the kept sessions in the sessions directory dir, by name. A missing file has none, and a line
// without a mode and a session is skipped.
func LoadKept(dir string) map[string]Kept {
	kept := map[string]Kept{}
	if dir == "" {
		return kept
	}
	data, err := os.ReadFile(filepath.Join(dir, keptFile))
	if err != nil {
		return kept
	}
	for line := range strings.Lines(string(data)) {
		f := strings.Split(strings.TrimRight(line, "\n"), "\t")
		if len(f) < 2 || f[0] == "" || f[1] == "" {
			continue
		}
		mode, since, _ := strings.Cut(f[0], ":")
		k := Kept{Mode: KeepMode(mode)}
		if secs, err := strconv.ParseInt(since, 10, 64); err == nil {
			k.Since = time.Unix(secs, 0)
		}
		if len(f) > 2 {
			k.Handles = f[2:]
		}
		kept[f[1]] = k
	}
	return kept
}

// saveKept writes kept to the sessions directory dir, replacing the file whole, or removes it when kept
// is empty.
func saveKept(dir string, kept map[string]Kept) error {
	path := filepath.Join(dir, keptFile)
	if len(kept) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(kept)) {
		k := kept[name]
		mode := string(k.Mode)
		if !k.Since.IsZero() {
			mode += ":" + strconv.FormatInt(k.Since.Unix(), 10)
		}
		b.WriteString(strings.Join(append([]string{mode, name}, k.Handles...), "\t") + "\n")
	}
	tmp, err := os.CreateTemp(dir, keptFile+".*")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// suspendedSessions names the sessions kept with KeepSuspended in the sessions directory dir, in order.
func suspendedSessions(dir string) []string {
	var names []string
	for name, k := range LoadKept(dir) {
		if k.Mode == KeepSuspended {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// keptHandles are what the provider kept of the kept sessions in dir, see Kept: a session of its that a
// handle names is a kept kitty session's, which that session's row stands for, not a detached one.
func keptHandles(dir string) map[string]bool {
	out := map[string]bool{}
	for _, k := range LoadKept(dir) {
		for _, h := range k.Handles {
			out[h] = true
		}
	}
	return out
}

// KeepSession closes the windows of session, keeping it as mode says, see Kept. It saves the session's
// file first, and refuses a session it saves no windows of. It does not wait for kitty to have closed the
// windows, see CloseSession, but has done everything else by the time it asks: the form that asks may be
// over one of them.
func KeepSession(cfg config.Config, session string, mode KeepMode) error {
	return keepSession(cfg, session, mode, true, nil)
}

// keepSession is KeepSession, asking the provider to keep what runs in the windows only with ask: closing
// with the provider's sessions ended, or left running, keeps the session's file alone. then, when not nil,
// runs once the file is saved and the windows marked, before they are closed: ending the provider's
// sessions there can close a window before kspaces does, as a multiplexer's client exits with its
// session, so the file has to be saved by then, and the window marked for the saver to leave it whole.
// What then fails stops nothing, and is returned with the rest.
func keepSession(cfg config.Config, session string, mode KeepMode, ask bool, then func() error) error {
	if !CanKeep(cfg) {
		return errors.New("keeping a session needs [kitty] save_sessions and a sessions directory")
	}
	all, err := kitty.ListTabs()
	if err != nil {
		return err
	}
	tabs, _ := sessionTabs(all, session, 0)
	if len(tabs) == 0 {
		return fmt.Errorf("%s has no windows open", session)
	}
	windows := sessionWindows(tabs, session)
	if err := kitty.SaveSessions(session); err != nil {
		return fmt.Errorf("saving %s: %w", session, err)
	}
	if !opensWindows(SessionFile(cfg, session)) {
		return fmt.Errorf("saving %s left no windows in %s", session, SessionFile(cfg, session))
	}
	var caps Capabilities
	if ask && len(cfg.Provider) > 0 {
		caps = ProviderCapabilities(cfg.Provider)
	}
	keeps := caps[CapKeep]
	// Listed before the provider is asked anything, and before the windows are marked, which leaves them
	// out of what the provider is told: from then on a list shows the sessions they show detached, and the
	// keep shelves them before it says what it kept. Listed as keeping, with those sessions, a list sees
	// them as this session's, which the row says it is keeping.
	line := Kept{Mode: mode}
	if keeps {
		line = Kept{Mode: mode.keeping(), Handles: shownHandles(cfg.Provider, caps, all, windows), Since: time.Now()}
	}
	if err := setKept(cfg.SessionsDir, session, &line); err != nil {
		return err
	}
	// Marked before the provider ends anything: a window whose program ends closes on its own, and the saver
	// and whatever ends a window's multiplexer session as it closes have to see it is kept by then.
	undo := func(err error) error {
		_ = kitty.SetSessionVar(session, kitty.KeepVar, "")
		return errors.Join(err, setKept(cfg.SessionsDir, session, nil))
	}
	if err := kitty.SetSessionVar(session, kitty.KeepVar, string(mode)); err != nil {
		return undo(err)
	}
	if keeps {
		handles, err := providerKeep(cfg.Provider, session, mode, windows)
		if err != nil {
			return undo(err)
		}
		if err := setKept(cfg.SessionsDir, session, &Kept{Mode: mode, Handles: handles}); err != nil {
			return err
		}
	}
	var done error
	if then != nil {
		done = then()
	}
	// Windows that went as their sessions ended are closed already: --ignore-no-match.
	return errors.Join(done, kitty.CloseSession(session))
}

// setKept lists session in the sessions directory dir as k, or not with k nil, the rest of the list as it
// is now: another kspaces may have listed a session since this one read it.
func setKept(dir, session string, k *Kept) error {
	kept := LoadKept(dir)
	delete(kept, session)
	if k != nil {
		kept[session] = *k
	}
	return saveKept(dir, kept)
}

// shownHandles are the ids and names of the provider's sessions windows show, of the windows of tabs, as
// it lists them now, none from a provider that does not list or fails to.
func shownHandles(provider []string, caps Capabilities, tabs []kitty.FullTab, windows []ProviderWindow) []string {
	if !caps[CapList] {
		return nil
	}
	st, err := listSessions(provider, fullWindows(tabs))
	if err != nil {
		logs.Error("listing the sessions to keep", err)
		return nil
	}
	var out []string
	for _, s := range st.Sessions {
		if slices.ContainsFunc(windows, func(w ProviderWindow) bool { return slices.Contains(s.Windows, w.ID) }) {
			out = append(out, s.ID, s.Name)
		}
	}
	return out
}

// unkeep drops session from the kept sessions and has the provider forget what it kept of it. What was
// kept with a provider that is no longer configured is left to it, and logged.
func unkeep(cfg config.Config, session string) error {
	if cfg.SessionsDir == "" {
		return nil
	}
	kept := LoadKept(cfg.SessionsDir)
	k, ok := kept[session]
	if !ok {
		return nil
	}
	switch {
	case len(k.Handles) == 0:
	case len(cfg.Provider) == 0:
		logs.Error("forgetting "+session, fmt.Errorf("no session provider is configured to forget %s", strings.Join(k.Handles, " ")))
	default:
		if err := providerForget(cfg.Provider, session, k.Handles); err != nil {
			return err
		}
	}
	delete(kept, session)
	return saveKept(cfg.SessionsDir, kept)
}

// CloseSessionAs closes session's windows, keeping it to open again as mode says, or not with mode empty,
// after doing fate to the provider's sessions its windows show, see ApplyClosingFate. Shelving a session
// kept to open again suspends it, as KeepSession with KeepSuspended does, so opening it brings them back;
// ending or leaving them keeps its file alone. Every way marks the windows kept, which tells a multiplexer's close watcher to leave
// what is left of them alone: nothing, once ended. Without a fate, as without a choice in the form, it
// closes as KeepSession does, or only closes the windows.
func CloseSessionAs(cfg config.Config, session string, mode KeepMode, fate Fate) error {
	if fate == "" {
		if mode == "" {
			return kitty.CloseSession(session)
		}
		return KeepSession(cfg, session, mode)
	}
	var caps Capabilities
	if fate != FateLeave {
		caps = ProviderCapabilities(cfg.Provider)
	}
	// Shelving a session kept to open again is suspending it: kept until it is opened again, and marked
	// suspended in the list, rather than looking closed with what ran in it shelved.
	if fate == FateShelve && mode != "" {
		mode = KeepSuspended
	}
	keeps := fate == FateShelve && mode != "" && caps[CapKeep]
	// Which sessions the windows show is asked first, before the windows are marked or any closes.
	var sessions []ProviderSession
	var listed error
	if !keeps && fate != FateLeave {
		sessions, listed = closingSessionsNow(cfg.Provider, caps, session, false)
	}
	end := func() error { return errors.Join(listed, ApplyFate(cfg.Provider, caps, fate, sessions)) }
	if mode != "" {
		return keepSession(cfg, session, mode, keeps, end)
	}
	if err := kitty.SetSessionVar(session, kitty.KeepVar, string(KeepClosed)); err != nil {
		return err
	}
	return errors.Join(end(), kitty.CloseSession(session))
}
