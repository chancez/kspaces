package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

// detachedFixture is the fixture with provider sessions no window shows: two in gr, one in docs, which is folded,
// and one in default.
func detachedFixture() spaces.Snapshot {
	snap := fixture(time.Now())
	snap.Config = &config.Config{Launch: []string{"launch"}, Provider: []string{"prov"}}
	snap.Caps = spaces.Capabilities{spaces.CapList: true, spaces.CapOpen: true}
	snap.Detached = map[string]spaces.Detached{
		"gr": {Hosts: []spaces.HostSessions{{Sessions: []spaces.ProviderSession{
			{ID: "a1", Name: "build", Space: "gr", State: "running", Busy: true, Command: "make watch", Tags: map[string]string{"worktree": "fix"}},
			{ID: "b2", Name: "@b2", Space: "gr", State: "shelved", Title: "claude"},
		}}}},
		"docs":    {Hosts: []spaces.HostSessions{{Sessions: []spaces.ProviderSession{{ID: "c3", Name: "notes", Space: "docs", State: "exited"}}}}},
		"default": {Hosts: []spaces.HostSessions{{Sessions: []spaces.ProviderSession{{ID: "d4", Name: "scratch", State: "running", Status: "blocked"}}}}},
	}
	return snap
}

// A space's provider sessions no window shows are a row after its own, of how many, but a folded space's;
// space shows them under it and hides them again. enter on one opens it in a tab of the sidebar's own
// session, which is the space's, running what the provider says opens it.
func TestDetachedRows(t *testing.T) {
	e := newSidebarEnv(t)
	calls := fakeKittenCalls(t)
	opens := fakeProvider(t, map[string]string{"open": `{"argv": ["mux", "attach", "b2"]}`}, "")
	e.m.Update(detachedFixture())
	e.m.Update(tea.FocusMsg{})
	head := "  \u25b6 2 detached"
	list := func(gr ...string) []string {
		return append(append([]string{
			spacesTitle,
			"",
			"  " + termIcon + "default",
			"  \u25b6 1 detached",
			"",
			"\u25bc " + repoIcon + "gr main",
			here + blocked + "dev pr/me/dev",
			"  " + branchIcon + "fix fix",
		}, gr...),
			"",
			"  "+repoIcon+"kitty master",
			"",
			"\u25b6 \uf110  docs main",
			"",
			"  \uf00d  broken",
		)
	}
	if got, want := e.spacesScreen(), list(head); !reflect.DeepEqual(got, want) {
		t.Errorf("collapsed:\n%q\nwant\n%q", got, want)
	}
	e.press("j", "j", "space")
	expanded := list("  \u25bc 2 detached", "    "+termIcon+"build busy make watch", "    "+termIcon+"@b2 shelved claude")
	if got := e.spacesScreen(); !reflect.DeepEqual(got, expanded) {
		t.Errorf("expanded:\n%q\nwant\n%q", got, expanded)
	}
	cmd := e.press("j", "j", "enter")
	if cmd == nil {
		t.Fatal("enter ran nothing")
	}
	cmd()
	want := [][]string{
		{"@", "focus-window", "--match", "id:11"},
		{"@", "action", "goto_session", "gr@dev.kitty-session"},
		{"@", "action", "--match=id:11", "launch", "--type=tab", "--cwd=/src/gr/.worktrees/dev", "--add-to-session=gr@dev",
			"--var=kspaces_space=gr", "--var=kspaces_worktree=dev", "mux", "attach", "b2"},
	}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("enter ran\n%q\nwant\n%q", got, want)
	}
	if got, want := opens(), []string{`open {"session":"b2"}`}; !reflect.DeepEqual(got, want) {
		t.Errorf("asked the provider %q, want %q", got, want)
	}

	// A click on the head hides them again, as space does.
	e.click(fmt.Sprintf(entryZone, slices.IndexFunc(e.m.rows, func(r spaces.Row) bool { return r.Detached != nil && r.Detached.Of.Name == "gr" })), tea.MouseLeft)
	if got, want := e.spacesScreen(), list(head); !reflect.DeepEqual(got, want) {
		t.Errorf("clicked:\n%q\nwant\n%q", got, want)
	}
	// What the filters look for is not there.
	e.press("O")
	for _, r := range e.m.rows {
		if r.Detached != nil {
			t.Errorf("a detached row under the open filter: %#v", r.Detached)
		}
	}
}

// fakeProvider puts a session provider called prov first on PATH, which records each call as its verb and
// its request, and prints out[VERB]; a request naming fail fails, its stderr saying so. It returns the
// calls.
func fakeProvider(t *testing.T, out map[string]string, fail string) func() []string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	script := "#!/bin/sh\nreq=$(/bin/cat)\necho \"$1 $req\" >> " + log + "\n"
	if fail != "" {
		script += "case \"$req\" in *'\"" + fail + "\"'*) echo \"no session " + fail + "\" >&2; exit 1;; esac\n"
	}
	for verb, text := range out {
		script += "if [ \"$1\" = " + verb + " ]; then echo '" + text + "'; fi\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "prov"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() []string {
		data, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}
}
