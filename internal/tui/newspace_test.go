package tui

import (
	"path/filepath"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

// The new space form refuses a name that is none and, on this machine, a directory that is none, before
// anything is made; it makes the space, in the directory given, absolute, or the home directory, and opens
// it.
func TestNewSpaceForm(t *testing.T) {
	home, _ := addSpaceEnv(t)
	cfg := config.Config{}
	f := newNewSpaceForm(cfg, "", "")
	f.Update(tea.WindowSizeMsg{Width: 90, Height: 20})
	screen := func(name, dir, says, buttons string) []string {
		return []string{"", "  New space on " + hostName(""), "", "  name       " + name, "  directory  " + dir, "", "  " + says, "", "   " + buttons, ""}
	}
	if got, want := formScreen(f), screen("the space's name", "the home directory", "Name the space, and where its windows start", "Make    Cancel esc   tab next field"); !reflect.DeepEqual(got, want) {
		t.Errorf("empty:\n%q\nwant\n%q", got, want)
	}
	type step struct {
		name, dir string
		want      []string
	}
	for _, s := range []step{
		{"bad name", "", screen("bad name", "the home directory", `"bad name": a space's name is letters, digits, '.', '_' and '-'`, "Make    Cancel esc   tab next field")},
		{"scratch", "~/nothing", screen("scratch", "~/nothing", "~/nothing is not a directory", "Make    Cancel esc   tab next field")},
	} {
		f = newNewSpaceForm(cfg, "", s.dir)
		f.Update(tea.WindowSizeMsg{Width: 90, Height: 20})
		typeInto(f, s.name)
		f.Update(keyMsg("enter"))
		if got := formScreen(f); !reflect.DeepEqual(got, s.want) || f.busy {
			t.Errorf("%q in %q: busy %v\n%q\nwant\n%q", s.name, s.dir, f.busy, got, s.want)
		}
	}

	type made struct {
		name, dir string
		opened    spaces.Worktree
	}
	for _, c := range []struct {
		host, dir string
		says      string
		want      made
	}{
		{"", "", "Makes scratch on " + hostName("") + ", starting in the home directory, and opens it", made{"scratch", "", spaces.Worktree{Session: "scratch"}}},
		{"", "~/plain", "Makes scratch on " + hostName("") + ", starting in ~/plain, and opens it", made{"scratch", filepath.Join(home, "plain"), spaces.Worktree{Session: "scratch", Path: filepath.Join(home, "plain")}}},
		// Another host's directory is the host's, as typed.
		{"white", "/srv/x", "Makes scratch on white, starting in /srv/x, and opens it", made{"scratch", "/srv/x", spaces.Worktree{Session: "white:scratch", Host: "white", Path: "/srv/x"}}},
	} {
		var got made
		f = newNewSpaceForm(cfg, c.host, c.dir)
		f.newSpace = func(name, dir string) error { got.name, got.dir = name, dir; return nil }
		f.open = func(wt spaces.Worktree) error { got.opened = wt; return nil }
		f.Update(tea.WindowSizeMsg{Width: 90, Height: 20})
		typeInto(f, "scratch")
		if says := formScreen(f)[6]; says != "  "+c.says {
			t.Errorf("on %q in %q it says %q, want %q", c.host, c.dir, says, c.says)
		}
		_, cmd := f.Update(keyMsg("enter"))
		if cmd == nil {
			t.Fatalf("enter on %q in %q ran nothing", c.host, c.dir)
		}
		if _, quit := f.Update(cmd()); quit == nil || quit() != tea.Quit() {
			t.Errorf("the form stayed after making: %v", f.err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("on %q in %q made %+v, want %+v", c.host, c.dir, got, c.want)
		}
	}
}
