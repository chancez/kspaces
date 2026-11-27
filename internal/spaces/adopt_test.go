package spaces

import (
	"os"
	"reflect"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// A worktree's orphans are its space's sessions no window shows, running or shelved, kept for no kitty
// session, that belong in it: by their tag, else the deepest worktree holding their directory, else the
// main worktree. Another host's are its copy's alone.
func TestOrphansOf(t *testing.T) {
	gr := Space{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{
		{Path: "/src/gr", Session: "gr"}, {Path: "/src/gr/.worktrees/dev", Session: "gr@dev"},
	}, Missing: []Worktree{{Session: "gr@gone"}}}
	tagged := ProviderSession{ID: "a", Space: "gr", State: "running", Tags: map[string]string{"worktree": "dev"}}
	inDev := ProviderSession{ID: "b", Space: "gr", State: "shelved", Cwd: "/src/gr/.worktrees/dev/web"}
	inRoot := ProviderSession{ID: "c", Space: "gr", State: "running", Cwd: "/src/gr/docs"}
	untagged := ProviderSession{ID: "d", Space: "gr", State: "running"}
	gone := ProviderSession{ID: "e", Space: "gr", State: "running", Tags: map[string]string{"worktree": "gone"}}
	shown := ProviderSession{ID: "f", Space: "gr", State: "running", Windows: []int{3}, Tags: map[string]string{"worktree": "dev"}}
	exited := ProviderSession{ID: "g", Space: "gr", State: "exited", Tags: map[string]string{"worktree": "dev"}}
	kept := ProviderSession{ID: "h", Space: "gr", State: "shelved", Tags: map[string]string{"worktree": "dev"}}
	other := ProviderSession{ID: "i", Space: "docs", State: "running"}
	remote := ProviderSession{ID: "white:@j", Space: "gr", Host: "white", State: "running", Tags: map[string]string{"worktree": "dev"}}
	all := []ProviderSession{tagged, inDev, inRoot, untagged, gone, shown, exited, kept, other, remote}
	keptSet := map[string]bool{"h": true}

	whiteGr := Space{Name: "gr", Root: "/home/me/gr", Worktrees: []Worktree{
		{Path: "/home/me/gr", Session: "white:gr", Host: "white"}, {Path: "/home/me/gr/.worktrees/dev", Session: "white:gr@dev", Host: "white"},
	}}
	got := map[string][]ProviderSession{
		"gr@dev":       orphansOf(gr, "gr@dev", all, keptSet),
		"gr":           orphansOf(gr, "gr", all, keptSet),
		"gr@gone":      orphansOf(gr, "gr@gone", all, keptSet),
		"white:gr@dev": orphansOf(whiteGr, "white:gr@dev", all, keptSet),
		"default":      orphansOf(Space{Name: "default", Worktrees: []Worktree{{Session: "default"}}}, "default", []ProviderSession{{ID: "k", State: "running"}}, nil),
	}
	want := map[string][]ProviderSession{
		"gr@dev":       {tagged, inDev},
		"gr":           {inRoot, untagged},
		"gr@gone":      {gone},
		"white:gr@dev": {remote},
		"default":      nil,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// Opening waits for the orphans to stay as they are, as the restored windows attach, then opens each left
// in a tab of the session beside its window, keeping focus.
func TestAdopt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		gr := Space{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Path: "/src/gr", Session: "gr"}}}
		restored := ProviderSession{ID: "@a", Name: "edit", Space: "gr", State: "running"}
		orphan := ProviderSession{ID: "@b", Name: "build", Space: "gr", State: "running", Cwd: "/src/gr/web"}
		lists := 0
		var launched [][]string
		a := adopter{
			cfg: config.Config{Launch: []string{"launch"}}, space: gr, session: "gr",
			now: time.Now, sleep: time.Sleep,
			list: func() (*ProviderState, error) {
				lists++
				r := restored
				if lists > 2 {
					// Its restored window's client attached.
					r.Windows = []int{7}
				}
				return &ProviderState{Sessions: []ProviderSession{r, orphan}}, nil
			},
			kept:   func() map[string]bool { return nil },
			anchor: func() int { return 7 },
			open: func(_ config.Config, s ProviderSession) ([]string, string, error) {
				return []string{"cm", "attach", s.ID}, "", nil
			},
			launch: func(session string, args []string) (int, error) {
				launched = append(launched, append([]string{session}, args...))
				return 9, nil
			},
		}
		start := time.Now()
		if err := a.run(); err != nil {
			t.Fatal(err)
		}
		want := [][]string{{"gr", "--match=id:7", "launch", "--type=tab", "--cwd=/src/gr/web", "--add-to-session=gr", "--var=kspaces_space=gr", "--keep-focus", "cm", "attach", "@b"}}
		if !reflect.DeepEqual(launched, want) {
			t.Errorf("launched\n%q\nwant\n%q", launched, want)
		}
		if took := time.Since(start); took != 3*adoptPoll+adoptSettle {
			t.Errorf("took %v, want %v: settling once the restored window attached", took, 3*adoptPoll+adoptSettle)
		}
	})
}

// Opening a worktree from no windows has a background kspaces adopt its orphans; switching to one with
// windows does not, nor opening one without a provider.
func TestOpenAdopts(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var got []bool
	for _, c := range []struct {
		provider []string
		windows  int
	}{{[]string{"prov"}, 0}, {[]string{"prov"}, 1}, {nil, 0}} {
		dir := t.TempDir()
		t.Setenv("PATH", dir+":/usr/bin:/bin")
		calls := fakeKittenLs(t, dir, "[]")
		cfg := config.Config{Provider: c.provider, SessionsDir: t.TempDir()}
		st := kitty.State{Windows: map[string]int{"notes": c.windows}, Sessions: map[string]int{}}
		if c.windows > 0 {
			st.Sessions["notes"] = 5
		}
		if err := OpenWorktree(cfg, st, Worktree{Session: "notes"}, OpenOptions{}); err != nil {
			t.Fatal(err)
		}
		got = append(got, slices.Contains(calls(), "@ launch --type=background --allow-remote-control "+exe+" adopt -- notes"))
	}
	if want := []bool{true, false, false}; !reflect.DeepEqual(got, want) {
		t.Errorf("adopted %v, want %v", got, want)
	}
}
