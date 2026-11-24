package spaces

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/chancez/kspaces/internal/config"
)

// fakeKeepTools puts a kitten and a keeper on PATH that record each call, a line of its words, in order,
// the keeper's stdin in the file it returns too. ls lists gr@dev's windows, one with a foreground program
// and a user variable, and a dock, and a window of gr. The saver's action writes gr@dev's file with a
// launch line, as save_sessions.py does. The keeper says it kept "k1" and "k2", or fails when keeperFails,
// and fails capabilities, as a program written for keeper does, or names caps when there are any.
func fakeKeepTools(t *testing.T, sessions string, keeperFails bool, caps ...string) (calls func() []string, stdin string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	stdin = filepath.Join(dir, "stdin")
	ls := `[{"tabs": [{"id": 1, "windows": [
		{"id": 10, "session_name": "gr@dev", "cmdline": ["/bin/zsh"], "cwd": "/src/gr/dev"},
		{"id": 11, "session_name": "gr@dev", "cmdline": ["nvim"], "cwd": "/src/gr/dev", "user_vars": {"mux": "m1"},
			"foreground_processes": [{"cmdline": ["nvim", "notes.md"]}]},
		{"id": 12, "session_name": "gr@dev", "cmdline": ["kspaces", "sidebar"], "user_vars": {"kspaces": "1"}},
		{"id": 20, "session_name": "gr", "cmdline": ["/bin/zsh"]}
	]}]}]`
	record := `echo "$(basename "$0") $*" >> ` + log + "\n"
	kitten := "#!/bin/sh\n" + record +
		"case \"$2\" in\n" +
		"ls) cat <<'EOF'\n" + ls + "\nEOF\n;;\n" +
		"action) printf 'new_tab\\nlaunch zsh\\n' > " + filepath.Join(sessions, "gr@dev.kitty-session") + ";;\n" +
		"esac\n"
	keep := `echo '{"kept": ["k1", "k2"]}'`
	if keeperFails {
		keep = "echo 'no mux server' >&2; exit 1"
	}
	capabilities := "echo 'unknown verb capabilities' >&2; exit 1"
	if len(caps) > 0 {
		capabilities = `echo '{"capabilities": ["` + strings.Join(caps, `", "`) + `"]}'; exit 0`
	}
	keeper := "#!/bin/sh\n" + record +
		"case \"$1\" in capabilities) " + capabilities + ";; esac\n" +
		"cat >> " + stdin + "\n" +
		"case \"$1\" in\n" +
		"keep) " + keep + ";;\n" +
		"esac\n"
	for name, script := range map[string]string{"kitten": kitten, "keeper": keeper} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	return func() []string {
		data, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	}, stdin
}

// readJSON is the JSON value in path, or nil when there is none.
func readJSON(t *testing.T, path string) any {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("%s: %v: %q", path, err, data)
	}
	return v
}

// Keeping a session saves its file, lists it as kept, marks its windows, and only then has the keeper keep
// what runs in them, told of the session's windows but kspaces' own, before closing them: a window whose
// program ends closes on its own, and has to be marked by then. What the keeper says it kept is listed with
// the session. When the keeper fails nothing is closed, and the mark and the listing are taken back.
// Without a keeper the layout alone is kept.
func TestKeepSession(t *testing.T) {
	windows := []any{
		map[string]any{"id": 10.0, "pid": 0.0, "cmdline": []any{"/bin/zsh"}, "foreground": []any{}, "user_vars": map[string]any{}, "cwd": "/src/gr/dev"},
		map[string]any{"id": 11.0, "pid": 0.0, "cmdline": []any{"nvim"}, "foreground": []any{[]any{"nvim", "notes.md"}}, "user_vars": map[string]any{"mux": "m1"}, "cwd": "/src/gr/dev"},
	}
	request := func(mode string) any {
		return map[string]any{"session": "gr@dev", "mode": mode, "windows": windows}
	}
	before := []string{
		"kitten @ ls",
		"kitten @ action kspaces_save_sessions 'gr@dev'",
	}
	cases := []struct {
		name        string
		mode        KeepMode
		keeper      []string
		keeperFails bool
		// What the keeper names as its capabilities, none for failing to.
		caps    []string
		calls   []string
		request any
		kept    map[string]Kept
		// The error KeepSession returns, empty for none.
		err string
	}{
		{
			name:   "suspended",
			mode:   KeepSuspended,
			keeper: []string{"keeper"},
			calls: append(before,
				"keeper capabilities",
				"kitten @ set-user-vars --match session:^gr@dev$ kspaces_keep=suspend",
				"keeper keep",
				"kitten @ close-window --ignore-no-match --match session:^gr@dev$",
			),
			request: request("suspend"),
			kept:    map[string]Kept{"gr": {Mode: KeepClosed, Handles: []string{"m3"}}, "gr@dev": {Mode: KeepSuspended, Handles: []string{"k1", "k2"}}},
		},
		{
			name:   "closed",
			mode:   KeepClosed,
			keeper: []string{"keeper"},
			calls: append(before,
				"keeper capabilities",
				"kitten @ set-user-vars --match session:^gr@dev$ kspaces_keep=close",
				"keeper keep",
				"kitten @ close-window --ignore-no-match --match session:^gr@dev$",
			),
			request: request("close"),
			kept:    map[string]Kept{"gr": {Mode: KeepClosed, Handles: []string{"m3"}}, "gr@dev": {Mode: KeepClosed, Handles: []string{"k1", "k2"}}},
		},
		{
			name:        "the keeper fails",
			mode:        KeepSuspended,
			keeper:      []string{"keeper"},
			keeperFails: true,
			calls: append(before,
				"keeper capabilities",
				"kitten @ set-user-vars --match session:^gr@dev$ kspaces_keep=suspend",
				"keeper keep",
				"kitten @ set-user-vars --match session:^gr@dev$ kspaces_keep",
			),
			request: request("suspend"),
			kept:    map[string]Kept{"gr": {Mode: KeepClosed, Handles: []string{"m3"}}},
			err:     "keeper keep: no mux server",
		},
		{
			name:   "a provider that does not keep",
			mode:   KeepSuspended,
			keeper: []string{"keeper"},
			caps:   []string{"list", "open"},
			calls: append(before,
				"keeper capabilities",
				"kitten @ set-user-vars --match session:^gr@dev$ kspaces_keep=suspend",
				"kitten @ close-window --ignore-no-match --match session:^gr@dev$",
			),
			kept: map[string]Kept{"gr": {Mode: KeepClosed, Handles: []string{"m3"}}, "gr@dev": {Mode: KeepSuspended}},
		},
		{
			name: "no keeper",
			mode: KeepSuspended,
			calls: append(before,
				"kitten @ set-user-vars --match session:^gr@dev$ kspaces_keep=suspend",
				"kitten @ close-window --ignore-no-match --match session:^gr@dev$",
			),
			kept: map[string]Kept{"gr": {Mode: KeepClosed, Handles: []string{"m3"}}, "gr@dev": {Mode: KeepSuspended}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, keptFile), []byte("close\tgr\tm3\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			calls, stdin := fakeKeepTools(t, dir, c.keeperFails, c.caps...)
			cfg := config.Config{SessionsDir: dir, Provider: c.keeper, Kitty: config.Kitty{SaveSessions: true}}
			err := KeepSession(cfg, "gr@dev", c.mode)
			if got := fmt.Sprint(err); err == nil && c.err != "" || err != nil && got != c.err {
				t.Fatalf("err %v, want %q", err, c.err)
			}
			if got := calls(); !reflect.DeepEqual(got, c.calls) {
				t.Errorf("calls\n%q\nwant\n%q", got, c.calls)
			}
			if got := readJSON(t, stdin); !reflect.DeepEqual(got, c.request) {
				t.Errorf("keeper was sent %v, want %v", got, c.request)
			}
			if got := LoadKept(dir); !reflect.DeepEqual(got, c.kept) {
				t.Errorf("kept %v, want %v", got, c.kept)
			}
		})
	}
}

// Without the session saver nothing writes the file a kept session opens from, so nothing is kept or closed.
func TestKeepSessionNeedsTheSaver(t *testing.T) {
	calls, _ := fakeKeepTools(t, t.TempDir(), false)
	cfg := config.Config{SessionsDir: t.TempDir(), Provider: []string{"keeper"}}
	if err := KeepSession(cfg, "gr@dev", KeepSuspended); err == nil {
		t.Error("kept a session without the saver")
	}
	if got := calls(); got != nil {
		t.Errorf("calls %q, want none", got)
	}
}

// Forgetting a kept session deletes its file, drops it from the kept sessions, and gives the keeper back
// what it kept to forget. A session kept with no handles needs no keeper.
func TestForgetKeptSession(t *testing.T) {
	for _, c := range []struct {
		name    string
		kept    string
		calls   []string
		request any
	}{
		{
			name:    "with handles",
			kept:    "suspend\tgr@dev\tk1\tk2\nclose\tgr\tm3\n",
			calls:   []string{"keeper forget"},
			request: map[string]any{"session": "gr@dev", "kept": []any{"k1", "k2"}},
		},
		{
			name: "without",
			kept: "suspend\tgr@dev\nclose\tgr\tm3\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			calls, stdin := fakeKeepTools(t, dir, false)
			for name, data := range map[string]string{keptFile: c.kept, "gr@dev.kitty-session": "launch zsh\n"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := ForgetSession(config.Config{SessionsDir: dir, Provider: []string{"keeper"}}, "gr@dev"); err != nil {
				t.Fatal(err)
			}
			if got := calls(); !reflect.DeepEqual(got, c.calls) {
				t.Errorf("calls %q, want %q", got, c.calls)
			}
			if got := readJSON(t, stdin); !reflect.DeepEqual(got, c.request) {
				t.Errorf("keeper was sent %v, want %v", got, c.request)
			}
			if got, want := LoadKept(dir), map[string]Kept{"gr": {Mode: KeepClosed, Handles: []string{"m3"}}}; !reflect.DeepEqual(got, want) {
				t.Errorf("kept %v, want %v", got, want)
			}
			if _, err := os.Stat(filepath.Join(dir, "gr@dev.kitty-session")); !os.IsNotExist(err) {
				t.Errorf("session file still there: %v", err)
			}
		})
	}
}

// Closing with the provider's sessions ended or left running keeps the session's file and marks its
// windows, but does not ask the provider to keep anything.
func TestKeepSessionWithoutTheProvider(t *testing.T) {
	dir := t.TempDir()
	calls, _ := fakeKeepTools(t, dir, false, "keep")
	cfg := config.Config{SessionsDir: dir, Provider: []string{"keeper"}, Kitty: config.Kitty{SaveSessions: true}}
	if err := keepSession(cfg, "gr@dev", KeepClosed, false, nil); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"kitten @ ls",
		"kitten @ action kspaces_save_sessions 'gr@dev'",
		"kitten @ set-user-vars --match session:^gr@dev$ kspaces_keep=close",
		"kitten @ close-window --ignore-no-match --match session:^gr@dev$",
	}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("calls\n%q\nwant\n%q", got, want)
	}
	if got, want := LoadKept(dir), map[string]Kept{"gr@dev": {Mode: KeepClosed}}; !reflect.DeepEqual(got, want) {
		t.Errorf("kept %v, want %v", got, want)
	}
}

// While the provider keeps a session, a list sees the sessions its windows showed as that session's, not
// detached: the provider here shelves them and takes its time saying what it kept, and before it does the
// session is listed as suspending, with their ids and names. Once it says, the session is listed as
// suspended with what it kept.
func TestKeepSessionWhileTheProviderKeeps(t *testing.T) {
	sessions := t.TempDir()
	dir, _ := fakeProvider(t, map[string]string{
		"capabilities": `{"capabilities": ["keep", "list", "shelve"]}`,
	}, nil)
	provider := filepath.Join(dir, "prov")
	started, shelved, release := filepath.Join(dir, "started"), filepath.Join(dir, "shelved"), filepath.Join(dir, "release")
	script := "#!/bin/sh\nd=" + dir + "\n/bin/cat > \"$d/$1.in\"\ncase \"$1\" in\n" +
		"capabilities) /bin/cat \"$d/capabilities.out\";;\n" +
		"list) if [ -f " + shelved + " ]; then echo '{\"sessions\": [{\"id\": \"a\", \"name\": \"build\", \"state\": \"shelved\", \"windows\": []}]}';\n" +
		"  else echo '{\"sessions\": [{\"id\": \"a\", \"name\": \"build\", \"state\": \"running\", \"windows\": [10]}]}'; fi;;\n" +
		"keep) : > " + shelved + "; : > " + started + "\n" +
		"  while [ ! -f " + release + " ]; do sleep 0.01; done\n" +
		"  echo '{\"kept\": [\"h-a\"]}';;\n" +
		"esac\n"
	if err := os.WriteFile(provider, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ls := `[{"tabs": [{"id": 1, "windows": [{"id": 10, "session_name": "cm@test", "cmdline": ["htop"]}]}]}]`
	kitten := "#!/bin/sh\ncase \"$2\" in\n" +
		"ls) /bin/cat <<'EOF'\n" + ls + "\nEOF\n;;\n" +
		"action) printf 'new_tab\\nlaunch htop\\n' > " + filepath.Join(sessions, "cm@test.kitty-session") + ";;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "kitten"), []byte(kitten), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	cfg := config.Config{Provider: []string{"prov"}, SessionsDir: sessions, Kitty: config.Kitty{SaveSessions: true}}
	done := make(chan error, 1)
	go func() { done <- KeepSession(cfg, "cm@test", KeepSuspended) }()
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the provider was never asked to keep")
		}
	}
	st, err := listSessions(cfg.Provider, nil)
	if err != nil {
		t.Fatal(err)
	}
	mid := LoadKept(sessions)
	if k := mid["cm@test"]; k.Since.IsZero() {
		t.Errorf("listed keeping with no time: %v", k)
	}
	for name, k := range mid {
		k.Since = time.Time{}
		mid[name] = k
	}
	got := []any{detachedBySpace(st, keptHandles(sessions)), mid}
	want := []any{map[string]Detached{}, map[string]Kept{"cm@test": {Mode: KeepSuspending, Handles: []string{"a", "build"}}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("while keeping\n%v\nwant\n%v", got, want)
	}
	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got, want := LoadKept(sessions), map[string]Kept{"cm@test": {Mode: KeepSuspended, Handles: []string{"h-a"}}}; !reflect.DeepEqual(got, want) {
		t.Errorf("kept %v, want %v", got, want)
	}
}
