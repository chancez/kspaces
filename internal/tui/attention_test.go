package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/status"
)

// fakeKittenCalls puts a kitten on PATH that records every call, and returns them, each its arguments.
func fakeKittenCalls(t *testing.T) func() [][]string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	script := "#!/bin/sh\necho \"$*\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "kitten"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	return func() [][]string {
		data, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		var out [][]string
		for line := range strings.Lines(string(data)) {
			out = append(out, strings.Fields(line))
		}
		return out
	}
}

// ! goes to the window that needs me most, through its session so the tab bar follows, wherever the
// cursor is; with nothing blocked, failed or done it does nothing.
func TestAttendKey(t *testing.T) {
	e := newSidebarEnv(t)
	calls := fakeKittenCalls(t)
	snap := fixture(time.Now())
	// The failed window, in gr, outranks the blocked one once that one is working.
	snap.Kitty.Statuses[0].State = "working"
	snap.Kitty.Statuses[1].Session = "gr"
	status.Sort(snap.Kitty.Statuses)
	e.m.Update(snap)
	e.m.Update(tea.FocusMsg{})
	e.press("G")
	cmd := e.press("!")
	if cmd == nil {
		t.Fatal("! ran nothing")
	}
	cmd()
	want := [][]string{
		{"@", "focus-window", "--match", "id:11"},
		{"@", "action", "goto_session", "gr.kitty-session"},
		{"@", "focus-window", "--match", "id:21"},
	}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("! ran\n%q\nwant\n%q", got, want)
	}

	snap.Kitty.Statuses = snap.Kitty.Statuses[1:]
	e.m.Update(snap)
	if cmd := e.press("!"); cmd != nil {
		t.Errorf("! with nothing to attend to ran %v", cmd())
	}
}
