package spaces

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
)

// keptFile, in the sessions directory, lists the kept sessions, a line each of tab-separated fields: the
// mode, the session, then what the provider kept of it. save_sessions.py reads it too: it keeps the file of a
// session listed there when its last window closes, and drops the session from the list once it has
// windows again.
const keptFile = ".kept-sessions"

// Kept is a session KeepSession kept.
type Kept struct {
	Mode KeepMode
	// What the provider said it kept, which it is given back to forget, see provider.go.
	Handles []string
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
		k := Kept{Mode: KeepMode(f[0])}
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
		b.WriteString(strings.Join(append([]string{string(k.Mode), name}, k.Handles...), "\t") + "\n")
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
	tabs, err := kitty.ListTabs()
	if err != nil {
		return err
	}
	tabs, _ = sessionTabs(tabs, session, 0)
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
	kept := LoadKept(cfg.SessionsDir)
	kept[session] = Kept{Mode: mode}
	if err := saveKept(cfg.SessionsDir, kept); err != nil {
		return err
	}
	// Marked before the provider ends anything: a window whose program ends closes on its own, and the saver
	// and whatever ends a window's multiplexer session as it closes have to see it is kept by then.
	undo := func(err error) error {
		_ = kitty.SetSessionVar(session, kitty.KeepVar, "")
		delete(kept, session)
		return errors.Join(err, saveKept(cfg.SessionsDir, kept))
	}
	if err := kitty.SetSessionVar(session, kitty.KeepVar, string(mode)); err != nil {
		return undo(err)
	}
	if ask && len(cfg.Provider) > 0 && ProviderCapabilities(cfg.Provider)[CapKeep] {
		handles, err := providerKeep(cfg.Provider, session, mode, windows)
		if err != nil {
			return undo(err)
		}
		kept[session] = Kept{Mode: mode, Handles: handles}
		if err := saveKept(cfg.SessionsDir, kept); err != nil {
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
// kept to open again is the provider's keep, so opening it brings them back; ending or leaving them keeps
// its file alone. Every way marks the windows kept, which tells a multiplexer's close watcher to leave
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
