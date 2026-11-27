package spaces

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chancez/kspaces/internal/config"
)

// An ad hoc space is named as cm names a space, and is not default nor a configured space.
func TestCheckAdHocName(t *testing.T) {
	cfg := config.Config{Spaces: []string{"/src/gr"}}
	got := map[string]bool{}
	for _, name := range []string{"scratch", "incident-123", ".dotfiles", "a.b_c", "", ".", "..", "has space", "a/b", "default", "gr", string(make([]byte, 65))} {
		got[name] = CheckAdHocName(cfg, name) == nil
	}
	want := map[string]bool{"scratch": true, "incident-123": true, ".dotfiles": true, "a.b_c": true, "": false, ".": false, "..": false,
		"has space": false, "a/b": false, "default": false, "gr": false, string(make([]byte, 65)): false}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("valid %v, want %v", got, want)
	}
}

// A host keeps its ad hoc spaces in a file, by name, so a server started again has them; a second of one
// name is refused, and one removed is gone. Each load carries them.
func TestAdHocSpaces(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	if err := os.MkdirAll(filepath.Join(cfgDir, "kspaces"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "kspaces", "kspaces.toml"), []byte("spaces = []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "state")
	h := NewLocalHost("", dir)
	h.load = func() HostData { return HostData{Config: &config.Config{}} }
	for _, s := range []AdHocSpace{{Name: "scratch"}, {Name: "incident", Dir: "/tmp/incident"}} {
		if err := h.adHoc.add(s); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.adHoc.add(AdHocSpace{Name: "scratch"}); !errors.Is(err, ErrSpaceExists) {
		t.Errorf("a second scratch: %v", err)
	}
	again := NewLocalHost("", dir)
	again.load = h.load
	hd, _ := again.Load(true)
	if want := []AdHocSpace{{Name: "incident", Dir: "/tmp/incident"}, {Name: "scratch"}}; !reflect.DeepEqual(hd.AdHoc, want) {
		t.Errorf("kept %+v, want %+v", hd.AdHoc, want)
	}
	if err := again.RemoveSpace("scratch"); err != nil {
		t.Fatal(err)
	}
	hd, _ = again.Load(true)
	if want := []AdHocSpace{{Name: "incident", Dir: "/tmp/incident"}}; !reflect.DeepEqual(hd.AdHoc, want) {
		t.Errorf("after removing scratch %+v, want %+v", hd.AdHoc, want)
	}
}

// This machine's ad hoc spaces are its sessions in no space, each starting in its directory, open or not.
func TestAdHocOthers(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sh := Shared{Config: &config.Config{}, AdHoc: []AdHocSpace{{Name: "scratch", Dir: "/tmp/scratch"}, {Name: "notes"}}}
	got := sh.View(0).Others
	want := []Space{
		{Name: "notes", Worktrees: []Worktree{{Session: "notes"}}},
		{Name: "scratch", Worktrees: []Worktree{{Session: "scratch", Path: "/tmp/scratch"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("others %+v, want %+v", got, want)
	}
}

// Another host's ad hoc spaces are its own sessions in no space, each a copy of its one kitty session, in
// its directory there; the provider's space of the host's sessions that is no space of its is one too, in
// its home directory, and its default is not listed.
func TestHostViewsAdHoc(t *testing.T) {
	sessions := []ProviderSession{
		{ID: "w:a", Space: "media", Host: "white"},
		{ID: "w:b", Space: "scratch", Host: "white"},
		{ID: "w:c", Host: "white"},
	}
	remotes := map[string]RemoteView{"white": {Loaded: true, AdHoc: []AdHocSpace{{Name: "scratch", Dir: "/srv/scratch"}, {Name: "empty"}}}}
	got := HostViews(nil, nil, sessions, nil, remotes)[1].Others
	want := []Space{
		{Name: "empty", Worktrees: []Worktree{{Session: "white:empty", Host: "white"}}},
		{Name: "media", Worktrees: []Worktree{{Session: "white:media", Host: "white"}}},
		{Name: "scratch", Worktrees: []Worktree{{Session: "white:scratch", Host: "white", Path: "/srv/scratch"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("white's others\n%+v\nwant\n%+v", got, want)
	}
}

// removingHost is a Host with ad hoc spaces, which records the spaces it removes.
type removingHost struct {
	fakeHost
	removed []string
}

func (h *removingHost) RemoveSpace(name string) error {
	h.removed = append(h.removed, name)
	return nil
}

// Removing another host's ad hoc space closes its copy's kitty session here and has its host remove it,
// ending its sessions there; removing this machine's, with no provider, closes its kitty session and has
// this machine's server forget it.
func TestRemoveAdHocSpace(t *testing.T) {
	path := os.Getenv("PATH")
	white := &removingHost{fakeHost: fakeHost{data: HostData{AdHoc: []AdHocSpace{{Name: "scratch"}}}}}
	UseRemotes(func(string, config.HostSettings) Host { return white })
	t.Cleanup(func() { UseRemotes(nil) })
	here := &removingHost{fakeHost: fakeHost{data: HostData{AdHoc: []AdHocSpace{{Name: "notes"}}}}}
	UseHost(here)
	t.Cleanup(func() { UseHost(nil) })
	cfg := config.Config{Hosts: map[string]config.HostSettings{"white": {}}}

	plans := map[string]SpaceRemoval{}
	var closed []string
	for _, ref := range []string{"white:scratch", "notes"} {
		// A kitty of the space's one window, which closing it empties.
		dir := t.TempDir()
		calls := fakeKittenLs(t, dir, `[{"tabs": [{"windows": [{"id": 10, "session_name": "`+ref+`"}]}]}]`)
		t.Setenv("PATH", dir+string(os.PathListSeparator)+path)
		defer func() {
			for _, c := range calls() {
				if strings.Contains(c, "close-window") {
					closed = append(closed, c)
				}
			}
		}()
		plan, err := PlanSpaceRemoval(cfg, ref)
		if err != nil {
			t.Fatal(err)
		}
		plans[ref] = plan
		if err := RemoveSpace(cfg, ref, FateEnd); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]SpaceRemoval{
		"white:scratch": {Ref: "white:scratch", Name: "scratch", Host: "white", AdHoc: true, Sessions: []string{"white:scratch"}, Fates: []Fate{FateEnd}},
		"notes":         {Ref: "notes", Name: "notes", AdHoc: true, Sessions: []string{"notes"}, Fates: []Fate{FateEnd}},
	}
	if !reflect.DeepEqual(plans, want) {
		t.Errorf("plans\n%+v\nwant\n%+v", plans, want)
	}
	if got := [][]string{white.removed, here.removed}; !reflect.DeepEqual(got, [][]string{{"scratch"}, {"notes"}}) {
		t.Errorf("removed %q", got)
	}
	t.Cleanup(func() {
		want := []string{"@ close-window --ignore-no-match --match session:^notes$", "@ close-window --ignore-no-match --match session:^white:scratch$"}
		if !reflect.DeepEqual(closed, want) {
			t.Errorf("closed %q, want %q", closed, want)
		}
	})
}
