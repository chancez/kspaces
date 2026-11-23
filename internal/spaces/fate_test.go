package spaces

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/kitty"
)

// There is a choice only with a provider that can do more than leave the sessions: End needs list and
// kill or remove, Shelve the keep of a session kept to open again or list and shelve.
func TestFates(t *testing.T) {
	got := [][]Fate{
		Fates(Capabilities{CapList: true, CapKill: true, CapRemove: true, CapShelve: true}, false),
		Fates(Capabilities{CapList: true, CapRemove: true}, false),
		Fates(Capabilities{CapKeep: true}, true),
		Fates(Capabilities{CapKeep: true}, false),
		Fates(Capabilities{CapKill: true, CapShelve: true}, false),
		Fates(nil, true),
	}
	want := [][]Fate{
		{FateEnd, FateShelve, FateLeave},
		{FateEnd, FateLeave},
		{FateShelve, FateLeave},
		nil, nil, nil,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

// A closing session's provider sessions are those its windows show, kspaces' own left out, and, for a
// worktree being removed, those of its space tagged with it.
func TestClosingSessions(t *testing.T) {
	data := []kitty.OSWindow{{Tabs: []kitty.Tab{{Windows: []kitty.Window{
		{ID: 1, SessionName: "gr@fix"}, {ID: 2, SessionName: "gr@fix"}, {ID: 3, SessionName: "gr"},
		{ID: 4, SessionName: "gr@fix", UserVars: map[string]string{"kspaces": "1"}},
	}}}}}
	a := ProviderSession{ID: "a", Space: "gr", Windows: []int{1}}
	b := ProviderSession{ID: "b", Space: "gr", Windows: []int{2, 3}}
	c := ProviderSession{ID: "c", Space: "gr", Windows: []int{3}}
	d := ProviderSession{ID: "d", Space: "gr", Tags: map[string]string{"worktree": "fix"}}
	e := ProviderSession{ID: "e", Space: "other", Tags: map[string]string{"worktree": "fix"}}
	f := ProviderSession{ID: "f", Space: "gr", Windows: []int{4}}
	st := &ProviderState{Sessions: []ProviderSession{a, b, c, d, e, f}}
	got := [][]ProviderSession{
		ClosingSessions(st, data, "gr@fix", true),
		ClosingSessions(st, data, "gr@fix", false),
		ClosingSessions(st, data, "gr", true),
		ClosingSessions(nil, data, "gr@fix", true),
	}
	want := [][]ProviderSession{{a, b, d}, {a, b}, {b, c}, nil}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

// Ending kills the running sessions and removes the ended ones, shelving shelves the running ones, and
// leaving does nothing; a verb the provider lacks is skipped, and one failing does not stop the rest.
func TestApplyFate(t *testing.T) {
	dir, calls := fakeProvider(t, nil, map[string]string{"kill": "no such session"})
	run := ProviderSession{ID: "r", Name: "build", State: "running"}
	ended := ProviderSession{ID: "e", Name: "old", State: "shelved"}
	all := Capabilities{CapKill: true, CapRemove: true, CapShelve: true}
	errs := []string{
		errText(ApplyFate([]string{"prov"}, all, FateEnd, []ProviderSession{run, ended})),
		errText(ApplyFate([]string{"prov"}, all, FateShelve, []ProviderSession{run, ended})),
		errText(ApplyFate([]string{"prov"}, all, FateLeave, []ProviderSession{run, ended})),
		errText(ApplyFate([]string{"prov"}, Capabilities{CapShelve: true}, FateEnd, []ProviderSession{run, ended})),
	}
	if want := []string{"build: prov kill: no such session", "<nil>", "<nil>", "<nil>"}; !reflect.DeepEqual(errs, want) {
		t.Errorf("errors %q, want %q", errs, want)
	}
	if got, want := calls(), []string{"kill", "remove", "shelve"}; !reflect.DeepEqual(got, want) {
		t.Errorf("called %q, want %q", got, want)
	}
	if got, want := sent(t, dir, "remove"), map[string]any{"session": "e"}; !reflect.DeepEqual(got, want) {
		t.Errorf("removed %v, want %v", got, want)
	}
}

// The forms say how many sessions a fate does, and what a busy one runs, which ending stops.
func TestFateWords(t *testing.T) {
	ss := []ProviderSession{
		{Name: "build", State: "running", Busy: true, Command: "make watch"},
		{Name: "idle", State: "running"},
		{Name: "quiet", State: "running", Busy: true},
	}
	got := []string{
		FateWords(FateEnd, ss), FateWords(FateShelve, ss[1:2]), FateWords(FateLeave, ss), FateWords(FateEnd, nil),
	}
	want := []string{
		"Ends 3 sessions: build running make watch, quiet running a command",
		"Shelves 1 session, to bring back later",
		"Leaves 3 sessions running",
		"",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// Closing a session ends what its windows show, as kitty and the provider list them now; a removed
// worktree's tagged sessions too.
func TestApplyClosingFate(t *testing.T) {
	dir, calls := fakeProvider(t, map[string]string{
		"capabilities": `{"capabilities": ["list", "kill", "remove"]}`,
		"list": `{"sessions": [{"id": "a", "space": "gr", "state": "running", "windows": [10]},
			{"id": "b", "space": "gr", "state": "exited", "tags": {"worktree": "fix"}},
			{"id": "c", "space": "gr", "state": "running", "windows": [20]}]}`,
	}, nil)
	ls := `[{"tabs": [{"windows": [{"id": 10, "session_name": "gr@fix"}, {"id": 20, "session_name": "gr"}]}]}]`
	kitten := "#!/bin/sh\n/bin/cat <<'EOF'\n" + ls + "\nEOF\n"
	if err := os.WriteFile(filepath.Join(dir, "kitten"), []byte(kitten), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	if err := ApplyClosingFate([]string{"prov"}, "gr@fix", FateEnd, true); err != nil {
		t.Fatal(err)
	}
	if got, want := calls(), []string{"capabilities", "list", "kill", "remove"}; !reflect.DeepEqual(got, want) {
		t.Errorf("called %q, want %q", got, want)
	}
	if got, want := []any{sent(t, dir, "kill"), sent(t, dir, "remove")}, []any{map[string]any{"session": "a"}, map[string]any{"session": "b"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("sent %v, want %v", got, want)
	}
}

func errText(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}
