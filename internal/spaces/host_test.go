package spaces

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/gitstate"
)

// The host asks git and gh, writes what they said, and a refresh has them asked again. A host started again
// carries on from the last one's pull requests. A load that is not fresh is the last, and subscribers hear
// of the loads that change something.
func TestLocalHostGitStates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run", "host.git.json")
	// UTC, as the states are read back from JSON: with the machine's zone at UTC that gives time.UTC, not
	// time.Local, and DeepEqual tells the two apart.
	now := time.Unix(1_000_000, 0).UTC()
	var asked []string
	watcher := func() *gitstate.Watcher {
		return gitstate.WatcherOf(
			func(dir string) (gitstate.Status, error) {
				asked = append(asked, "status "+dir)
				return gitstate.Status{Changes: 2}, nil
			},
			func(root, branch string) (*gh.PR, error) {
				asked = append(asked, "pr "+branch)
				return &gh.PR{Number: 7, State: "merged"}, nil
			},
			func() time.Time { return now },
			func(f func()) { f() },
		)
	}
	spaces := []sharedSpace{
		{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Path: "/src/gr", Branch: "main", Session: "gr"}, {Path: "/src/gr/dev", Branch: "dev", Session: "gr@dev"}}},
		{Name: "broken", Root: "/src/broken", Err: "not a git repository"},
	}
	host := func() *LocalHost {
		h := NewLocalHost(path, "")
		h.load = func() HostData { return HostData{Config: &config.Config{}, Spaces: spaces} }
		h.newWatcher = watcher
		return h
	}
	ha, hb := host(), host()
	var heard []map[string]gitstate.State
	stop := ha.Subscribe(func(hd HostData) { heard = append(heard, hd.Git) })
	load := func(f func() error) []string {
		asked = nil
		if err := f(); err != nil {
			t.Fatal(err)
		}
		return asked
	}
	loadA := func() error { _, err := ha.Load(true); return err }
	states := map[string]gitstate.State{
		"/src/gr":     {Status: &gitstate.Status{Changes: 2}, PR: &gh.PR{Number: 7, State: "merged"}, PRAt: now, PRBranch: "main"},
		"/src/gr/dev": {Status: &gitstate.Status{Changes: 2}, PR: &gh.PR{Number: 7, State: "merged"}, PRAt: now, PRBranch: "dev"},
	}
	got := [][]string{load(loadA)}
	if written, _ := readGit(path); !reflect.DeepEqual(written, states) {
		t.Errorf("written states:\n%+v\nwant\n%+v", written, states)
	}
	now = now.Add(time.Second)
	got = append(got, load(loadA), load(func() error { _, err := ha.Load(false); return err }), load(ha.Refresh), load(loadA))
	now = now.Add(time.Minute)
	got = append(got, load(func() error { _, err := hb.Load(false); return err }))
	want := [][]string{
		{"status /src/gr", "status /src/gr/dev", "pr main", "pr dev"},
		nil,
		nil,
		nil,
		{"status /src/gr", "status /src/gr/dev", "pr main", "pr dev"},
		// Statuses are read again, but the pull requests are not due.
		{"status /src/gr", "status /src/gr/dev"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("asked:\n%q\nwant\n%q", got, want)
	}
	// The first load, and the one after the refresh, whose pull requests were asked again; the others
	// found what the last had, and the one after stop is not heard.
	stop()
	now = now.Add(time.Minute)
	ha.Load(true)
	again := map[string]gitstate.State{}
	for path, st := range states {
		st.PRAt = st.PRAt.Add(time.Second)
		again[path] = st
	}
	if want := []map[string]gitstate.State{states, again}; !reflect.DeepEqual(heard, want) {
		t.Errorf("heard\n%+v\nwant\n%+v", heard, want)
	}
}
