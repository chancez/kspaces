package gitstate

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/chancez/kspaces/internal/gh"
)

func TestParseStatus(t *testing.T) {
	for name, tc := range map[string]struct {
		in   string
		want Status
	}{
		"ahead and behind, with changes": {
			in: "# branch.oid 1111111111111111111111111111111111111111\n# branch.head dev\n# branch.upstream origin/dev\n# branch.ab +2 -1\n" +
				"1 .M N... 100644 100644 100644 aaa bbb main.go\n" +
				"2 R. N... 100644 100644 100644 aaa bbb R100 new.go\told.go\n" +
				"u UU N... 100644 100644 100644 100644 aaa bbb ccc both.go\n" +
				"? scratch.txt\n",
			want: Status{Changes: 4, Upstream: "origin/dev", Ahead: 2, Behind: 1},
		},
		"clean, even with its upstream": {
			in:   "# branch.oid 1111111111111111111111111111111111111111\n# branch.head main\n# branch.upstream origin/main\n# branch.ab +0 -0\n",
			want: Status{Upstream: "origin/main"},
		},
		"an upstream that is gone": {
			in:   "# branch.oid 1111111111111111111111111111111111111111\n# branch.head fix\n# branch.upstream origin/fix\n? notes\n",
			want: Status{Changes: 1, Upstream: "origin/fix", Gone: true},
		},
		"no upstream": {
			in:   "# branch.oid 1111111111111111111111111111111111111111\n# branch.head local\n",
			want: Status{},
		},
		"detached": {
			in:   "# branch.oid 1111111111111111111111111111111111111111\n# branch.head (detached)\n1 M. N... 100644 100644 100644 aaa bbb a\n",
			want: Status{Changes: 1},
		},
	} {
		if got := ParseStatus(tc.in); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", name, got, tc.want)
		}
	}
}

// watchEnv is a watcher whose passes run at once, on a clock the test moves, recording what it asks.
type watchEnv struct {
	w      *Watcher
	now    time.Time
	asked  []string
	prs    map[string]*gh.PR
	failPR bool
}

func newWatchEnv() *watchEnv {
	e := &watchEnv{now: time.Unix(1_000_000, 0), prs: map[string]*gh.PR{"dev": {Number: 7, State: "open"}}}
	e.w = WatcherOf(
		func(dir string) (Status, error) {
			e.asked = append(e.asked, "status "+dir)
			if dir == "/src/broken" {
				return Status{}, errors.New("not a git repository")
			}
			return Status{Changes: 1}, nil
		},
		func(root, branch string) (*gh.PR, error) {
			e.asked = append(e.asked, "pr "+branch)
			if e.failPR {
				return nil, errors.New("gh: not logged in")
			}
			return e.prs[branch], nil
		},
		func() time.Time { return e.now },
		func(f func()) { f() },
	)
	return e
}

// ask is the questions an Update after d asked, then what the watcher knows after them.
func (e *watchEnv) ask(d time.Duration, targets ...Target) []string {
	e.now = e.now.Add(d)
	e.asked = nil
	e.w.Update(targets)
	return e.asked
}

// Statuses are read every ten seconds, pull requests looked up every three minutes and when the branch
// changes; a refresh asks again at once. A failure is nothing known, and keeps a known pull request.
func TestWatcher(t *testing.T) {
	e := newWatchEnv()
	dev := Target{Root: "/src/gr", Path: "/src/gr/dev", Branch: "dev"}
	broken := Target{Root: "/src/broken", Path: "/src/broken", Branch: "main"}
	got := [][]string{
		e.ask(0, dev, broken),
		e.ask(5*time.Second, dev, broken),
		e.ask(5*time.Second, dev, broken),
		e.ask(3*time.Minute, dev),
	}
	want := [][]string{
		{"status /src/gr/dev", "status /src/broken", "pr dev", "pr main"},
		nil,
		{"status /src/gr/dev", "status /src/broken"},
		{"status /src/gr/dev", "pr dev"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("asked:\n%q\nwant\n%q", got, want)
	}
	at := e.now
	if got, want := e.w.Update([]Target{dev}), map[string]State{"/src/gr/dev": {Status: &Status{Changes: 1}, PR: &gh.PR{Number: 7, State: "open"}, PRAt: at, PRBranch: "dev"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("states %+v, want %+v", got, want)
	}

	// gh failing keeps the pull request it knew of the branch, and a new branch is looked up at once.
	e.failPR = true
	e.w.Refresh()
	got = [][]string{e.ask(time.Second, dev)}
	if st := e.w.Update([]Target{dev})["/src/gr/dev"]; !reflect.DeepEqual(st.PR, &gh.PR{Number: 7, State: "open"}) {
		t.Errorf("after a failure: %+v", st.PR)
	}
	dev.Branch = "dev2"
	got = append(got, e.ask(time.Second, dev))
	if want := [][]string{{"status /src/gr/dev", "pr dev"}, {"pr dev2"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("asked:\n%q\nwant\n%q", got, want)
	}
	if st := e.w.Update([]Target{dev})["/src/gr/dev"]; st.PR != nil || st.PRBranch != "dev2" {
		t.Errorf("a new branch kept the old one's pull request: %+v", st)
	}

	// Seeded from another watcher, pull requests are due when they were there, and statuses at once.
	e2 := newWatchEnv()
	e2.now = e.now
	e2.w.Seed(e.w.Update([]Target{dev}))
	if got, want := e2.ask(time.Minute, dev), []string{"status /src/gr/dev"}; !reflect.DeepEqual(got, want) {
		t.Errorf("seeded watcher asked %q, want %q", got, want)
	}
}

// A PR is known of the branch gh was asked about, none included, and not of another branch or before gh
// was asked.
func TestKnownPR(t *testing.T) {
	at := time.Unix(1_000_000, 0)
	type known struct {
		PR *gh.PR
		OK bool
	}
	var got []known
	for _, st := range []State{
		{PR: &gh.PR{Number: 7, State: "open"}, PRAt: at, PRBranch: "dev"},
		{PRAt: at, PRBranch: "dev"},
		{PR: &gh.PR{Number: 7, State: "open"}, PRAt: at, PRBranch: "old"},
		{},
	} {
		pr, ok := st.KnownPR("dev")
		got = append(got, known{pr, ok})
	}
	want := []known{{&gh.PR{Number: 7, State: "open"}, true}, {nil, true}, {}, {}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}
