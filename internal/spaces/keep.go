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
// tabs and splits, which goto_session opens. What runs in its windows is the keeper's to keep, a program
// the config names, see keeper: one running each window in a multiplexer session keeps that session, and
// the window attaching to it again when the file reopens it brings it back. Without a keeper the layout
// alone is kept.

// KeepMode is how KeepSession keeps a session.
type KeepMode string

const (
	// KeepClosed keeps the session to open again: its file, and what the keeper keeps for a while, as the
	// keeper decides. What closing a session does.
	KeepClosed KeepMode = "close"
	// KeepSuspended keeps the session as KeepClosed does, and what the keeper keeps until the session is
	// opened again or forgotten.
	KeepSuspended KeepMode = "suspend"
)

// keptFile, in the sessions directory, lists the kept sessions, a line each of tab-separated fields: the
// mode, the session, then what the keeper kept of it. save_sessions.py reads it too: it keeps the file of a
// session listed there when its last window closes, and drops the session from the list once it has
// windows again.
const keptFile = ".kept-sessions"

// Kept is a session KeepSession kept.
type Kept struct {
	Mode KeepMode
	// What the keeper said it kept, which it is given back to forget, see keeper.
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

// KeepSession closes the windows of session, keeping it as mode says, see Kept. It saves the session's
// file first, and refuses a session it saves no windows of. It does not wait for kitty to have closed the
// windows, see CloseSession, but has done everything else by the time it asks: the form that asks may be
// over one of them.
func KeepSession(cfg config.Config, session string, mode KeepMode) error {
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
	windows := keeperWindows(tabs, session)
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
	// Marked before the keeper ends anything: a window whose program ends closes on its own, and the saver
	// and whatever ends a window's multiplexer session as it closes have to see it is kept by then.
	undo := func(err error) error {
		_ = kitty.SetSessionVar(session, kitty.KeepVar, "")
		delete(kept, session)
		return errors.Join(err, saveKept(cfg.SessionsDir, kept))
	}
	if err := kitty.SetSessionVar(session, kitty.KeepVar, string(mode)); err != nil {
		return undo(err)
	}
	if len(cfg.Keeper) > 0 {
		handles, err := keeperKeep(cfg.Keeper, session, mode, windows)
		if err != nil {
			return undo(err)
		}
		kept[session] = Kept{Mode: mode, Handles: handles}
		if err := saveKept(cfg.SessionsDir, kept); err != nil {
			return err
		}
	}
	return kitty.CloseSession(session)
}

// unkeep drops session from the kept sessions and has the keeper forget what it kept of it. What was kept
// with a keeper that is no longer configured is left to it, and logged.
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
	case len(cfg.Keeper) == 0:
		logs.Error("forgetting "+session, fmt.Errorf("no keeper is configured to forget %s", strings.Join(k.Handles, " ")))
	default:
		if err := keeperForget(cfg.Keeper, session, k.Handles); err != nil {
			return err
		}
	}
	delete(kept, session)
	return saveKept(cfg.SessionsDir, kept)
}
