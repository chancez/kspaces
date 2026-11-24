package spaces

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chancez/kspaces/internal/config"
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

// fakeKittenLs puts a kitten in dir, which is on PATH, that records each call and lists ls for ls, and no
// windows once a close-window has closed them.
func fakeKittenLs(t *testing.T, dir, ls string) func() []string {
	t.Helper()
	log, closed := filepath.Join(dir, "kitten-calls"), filepath.Join(dir, "kitten-closed")
	script := "#!/bin/sh\necho \"$*\" >> " + log + "\ncase \"$2\" in\nclose-window) : > " + closed + ";;\n" +
		"ls) if [ -f " + closed + " ]; then echo '[]'; else /bin/cat <<'EOF'\n" + ls + "\nEOF\nfi;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "kitten"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	return func() []string {
		data, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		var out []string
		for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			out = append(out, l)
		}
		return out
	}
}

// Closing a session not kept to open again ends what its windows show, shelves it, or leaves it running;
// every way marks the windows kept before they close, which tells a close watcher to leave them alone.
func TestCloseSessionAs(t *testing.T) {
	ls := `[{"tabs": [{"windows": [{"id": 10, "session_name": "gr@dev"}]}]}]`
	var got [][]string
	for _, fate := range []Fate{FateEnd, FateShelve, FateLeave} {
		dir, provider := fakeProvider(t, map[string]string{
			"capabilities": `{"capabilities": ["list", "kill", "shelve"]}`,
			"list":         `{"sessions": [{"id": "a", "state": "running", "windows": [10]}]}`,
		}, nil)
		kitten := fakeKittenLs(t, dir, ls)
		if err := CloseSessionAs(config.Config{Provider: []string{"prov"}}, "gr@dev", "", fate); err != nil {
			t.Fatal(err)
		}
		got = append(got, append(provider(), kitten()...))
	}
	marked := []string{"@ set-user-vars --match session:^gr@dev$ kspaces_keep=close", "@ close-window --ignore-no-match --match session:^gr@dev$"}
	want := [][]string{
		append([]string{"capabilities", "list", "kill", "@ ls"}, marked...),
		append([]string{"capabilities", "list", "shelve", "@ ls"}, marked...),
		marked,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// fakeVanishing puts a provider and a kitten on PATH where the provider's kill takes its session's window
// with it, as a multiplexer's client exits with its session: kitty lists no window after, and set-user-vars
// fails with none to match, as kitty's does. The session's saver writes its file. It returns the log of
// both, a line each, in order, and the sessions directory.
func fakeVanishing(t *testing.T) (func() string, string) {
	t.Helper()
	sessions := t.TempDir()
	dir, _ := fakeProvider(t, map[string]string{
		"capabilities": `{"capabilities": ["keep", "list", "kill", "shelve"]}`,
		"list":         `{"sessions": [{"id": "a", "state": "running", "windows": [10]}]}`,
	}, nil)
	log, gone := filepath.Join(dir, "order"), filepath.Join(dir, "gone")
	ls := `[{"tabs": [{"id": 1, "windows": [{"id": 10, "session_name": "cm@test", "cmdline": ["htop"]}]}]}]`
	provider := filepath.Join(dir, "prov")
	data, _ := os.ReadFile(provider)
	script := strings.Replace(string(data), "/bin/cat > \"$d/$1.in\"\n", "/bin/cat > \"$d/$1.in\"\necho \"prov $1\" >> "+log+"\nif [ \"$1\" = kill ]; then : > "+gone+"; fi\n", 1)
	if err := os.WriteFile(provider, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	kitten := "#!/bin/sh\necho \"kitten $2\" >> " + log + "\ncase \"$2\" in\n" +
		"ls) if [ -f " + gone + " ]; then echo '[]'; else /bin/cat <<'EOF'\n" + ls + "\nEOF\nfi;;\n" +
		"set-user-vars) if [ -f " + gone + " ]; then echo 'No matching windows' >&2; exit 1; fi;;\n" +
		"action) printf 'new_tab\\nlaunch htop\\n' > " + filepath.Join(sessions, "cm@test.kitty-session") + ";;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "kitten"), []byte(kitten), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	return func() string { data, _ := os.ReadFile(log); return string(data) }, sessions
}

// Ending a closed session's sessions makes their windows go on their own: the session's file is saved and
// its windows marked before that, so closing still keeps it whole to open again, and the windows already
// gone are no error.
func TestCloseSessionAsEndsBeforeClosing(t *testing.T) {
	ran, sessions := fakeVanishing(t)
	cfg := config.Config{Provider: []string{"prov"}, SessionsDir: sessions, Kitty: config.Kitty{SaveSessions: true}}
	if err := CloseSessionAs(cfg, "cm@test", KeepClosed, FateEnd); err != nil {
		t.Fatal(err)
	}
	want := "prov capabilities\nkitten ls\nprov list\nkitten ls\nkitten action\nkitten set-user-vars\nprov kill\nkitten close-window\n"
	if got := ran(); got != want {
		t.Errorf("ran\n%s\nwant\n%s", got, want)
	}
	if got, want := LoadKept(sessions), map[string]Kept{"cm@test": {Mode: KeepClosed}}; !reflect.DeepEqual(got, want) {
		t.Errorf("kept %v, want %v", got, want)
	}
	if !opensWindows(SessionFile(cfg, "cm@test")) {
		t.Error("the session's file opens no windows")
	}
}

// Shelving a session kept to open again suspends it, as Suspend does: the provider keeps it as suspended,
// its windows are marked suspended, and the kept list says suspended, which the list marks it with.
func TestCloseSessionAsShelveSuspends(t *testing.T) {
	ran, sessions := fakeVanishing(t)
	cfg := config.Config{Provider: []string{"prov"}, SessionsDir: sessions, Kitty: config.Kitty{SaveSessions: true}}
	if err := CloseSessionAs(cfg, "cm@test", KeepClosed, FateShelve); err != nil {
		t.Fatal(err)
	}
	want := "prov capabilities\nkitten ls\nkitten action\nkitten set-user-vars\nprov capabilities\nprov keep\nkitten close-window\n"
	if got := ran(); got != want {
		t.Errorf("ran\n%s\nwant\n%s", got, want)
	}
	got := []any{LoadKept(sessions), suspendedSessions(sessions)}
	if want := []any{map[string]Kept{"cm@test": {Mode: KeepSuspended}}, []string{"cm@test"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("kept %v, want %v", got, want)
	}
}

// Closing without the saver, and removing, end the sessions as well with their windows going first.
func TestEndingWindowsGoFirst(t *testing.T) {
	ran, _ := fakeVanishing(t)
	if err := CloseSessionAs(config.Config{Provider: []string{"prov"}}, "cm@test", "", FateEnd); err != nil {
		t.Errorf("closing: %v", err)
	}
	if got, want := ran(), "prov capabilities\nkitten ls\nprov list\nkitten set-user-vars\nprov kill\nkitten close-window\n"; got != want {
		t.Errorf("closing ran\n%s\nwant\n%s", got, want)
	}
	ran, sessions := fakeVanishing(t)
	if err := EndSession(config.Config{Provider: []string{"prov"}, SessionsDir: sessions}, "cm@test"); err != nil {
		t.Errorf("removing: %v", err)
	}
	if got, want := ran(), "prov capabilities\nkitten ls\nprov list\nprov kill\nkitten close-window\nkitten ls\n"; got != want {
		t.Errorf("removing ran\n%s\nwant\n%s", got, want)
	}
}
