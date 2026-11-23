package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
)

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

// Killing one session says what it runs, which goes; y has the provider kill it by its id.
func TestEndSessionsForm(t *testing.T) {
	calls := fakeProvider(t, nil, "")
	f := newEndSessionsForm([]string{"prov"}, "", []spaces.ProviderSession{{ID: "a1", Name: "build", State: "running", Busy: true, Command: "make watch"}})
	f.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	want := []string{"", "  Kill session build", "", "  build  busy; make watch running, killed", "", "   Kill y    Cancel n", ""}
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("form:\n%q\nwant\n%q", got, want)
	}
	_, end := f.Update(keyMsg("y"))
	if _, quit := f.Update(end()); quit == nil {
		t.Fatalf("the form stayed: %v", f.err)
	}
	if got, want := calls(), []string{`kill {"session":"a1"}`}; !reflect.DeepEqual(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
}

// Cleaning up a space's sessions ticks only those that exited or died; a ticked running one is killed, a
// ticked ended one removed. One the provider refuses says so in the form, which stays, and the rest still go.
func TestCleanupSessionsForm(t *testing.T) {
	calls := fakeProvider(t, nil, "e5")
	f := newEndSessionsForm([]string{"prov"}, "gr", []spaces.ProviderSession{
		{ID: "a1", Name: "build", State: "running", Busy: true, Command: "make watch"},
		{ID: "b2", Name: "kitty.2032", State: "running"},
		{ID: "c3", Name: "old", State: "exited", Title: "vim"},
		{ID: "d4", Name: "@d4", State: "shelved"},
		{ID: "e5", Name: "gone", State: "dead"},
	})
	f.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	lines := func(rows ...string) []string {
		return append(append([]string{"", "  Clean up the sessions of gr", "",
			"  Killing a running session ends it, and removing an ended one discards it: neither",
			"  comes back.", ""}, rows...), "")
	}
	want := lines(
		"  \u2610 build       busy; kept; make watch",
		"  \u2610 kitty.2032  running; kept",
		"  \u2611 old         exited; removed; vim",
		"  \u2610 @d4         shelved; kept",
		"  \u2611 gone        dead; removed",
		"",
		"   End 2 sessions y    Cancel n",
	)
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("form:\n%q\nwant\n%q", got, want)
	}
	for _, k := range []string{"j", "space", "j", "j", "space"} {
		f.Update(keyMsg(k))
	}
	_, end := f.Update(keyMsg("y"))
	if _, quit := f.Update(end()); quit != nil {
		t.Fatal("the form left with a session the provider refused")
	}
	want = lines(
		"  \u2610 build       busy; kept; make watch",
		"  \u2611 kitty.2032  running; killed",
		"  \u2611 old         exited; removed; vim",
		"  \u2611 @d4         shelved; removed",
		"  \u2611 gone        dead; removed",
		"",
		"  gone: prov remove: no session e5",
		"",
		"   End 4 sessions y    Cancel n",
	)
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("after:\n%q\nwant\n%q", got, want)
	}
	if got, want := calls(), []string{`kill {"session":"b2"}`, `remove {"session":"c3"}`, `remove {"session":"d4"}`, `remove {"session":"e5"}`}; !reflect.DeepEqual(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
}
