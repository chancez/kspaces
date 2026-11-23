package spaces

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chancez/kspaces/internal/kitty"
)

// fakeProvider puts a session provider on PATH, called prov, speaking the protocol from files: for each
// verb it saves its stdin as VERB.in in dir and records the verb, then prints out[VERB], or fails with
// fails[VERB] on its stderr. It returns dir and the verbs in the order called.
func fakeProvider(t *testing.T, out, fails map[string]string) (string, func() []string) {
	t.Helper()
	dir := t.TempDir()
	for verb, text := range out {
		if err := os.WriteFile(filepath.Join(dir, verb+".out"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for verb, text := range fails {
		if err := os.WriteFile(filepath.Join(dir, verb+".err"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	script := "#!/bin/sh\nd=" + dir + "\n/bin/cat > \"$d/$1.in\"\necho \"$1\" >> \"$d/calls\"\n" +
		"if [ -f \"$d/$1.err\" ]; then /bin/cat \"$d/$1.err\" >&2; exit 1; fi\n" +
		"if [ -f \"$d/$1.out\" ]; then /bin/cat \"$d/$1.out\"; fi\n"
	if err := os.WriteFile(filepath.Join(dir, "prov"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/bin:/usr/bin")
	return dir, func() []string {
		data, err := os.ReadFile(filepath.Join(dir, "calls"))
		if err != nil {
			return nil
		}
		return strings.Fields(string(data))
	}
}

// sent is the request the fake provider in dir was last sent for verb, decoded.
func sent(t *testing.T, dir, verb string) any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, verb+".in"))
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("%s sent %q: %v", verb, data, err)
	}
	return v
}

// A provider says what it does; one that fails to, or prints nothing, keeps alone, as a keeper does.
func TestProviderCapabilities(t *testing.T) {
	fakeProvider(t, map[string]string{"capabilities": `{"capabilities": ["list", "open", "kill"]}`}, nil)
	got := []Capabilities{ProviderCapabilities([]string{"prov"})}
	fakeProvider(t, nil, map[string]string{"capabilities": "unknown verb capabilities"})
	got = append(got, ProviderCapabilities([]string{"prov"}))
	fakeProvider(t, nil, nil)
	got = append(got, ProviderCapabilities([]string{"prov"}), ProviderCapabilities(nil))
	want := []Capabilities{{CapList: true, CapOpen: true, CapKill: true}, {CapKeep: true}, {CapKeep: true}, {}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

// list sends the windows and reads the sessions and spaces.
func TestListSessions(t *testing.T) {
	dir, _ := fakeProvider(t, map[string]string{"list": `{"spaces": ["default", "gr"], "sessions": [
		{"id": "a1", "name": "build", "space": "gr", "state": "running", "busy": true, "title": "vim", "command": "vim x",
		 "cwd": "/src/gr", "status": "blocked", "tags": {"worktree": "fix"}, "windows": [3], "extra": 1},
		{"id": "b2", "name": "old", "state": "shelved", "host": "server"}]}`}, nil)
	windows := []ProviderWindow{{ID: 3, PID: 99, Cmdline: []string{"mux", "attach"}, Foreground: [][]string{{"vim", "x"}}, UserVars: map[string]string{"k": "v"}, Cwd: "/src"}}
	got, err := listSessions([]string{"prov"}, windows)
	want := &ProviderState{
		Spaces: []string{"default", "gr"},
		Sessions: []ProviderSession{
			{ID: "a1", Name: "build", Space: "gr", State: "running", Busy: true, Title: "vim", Command: "vim x", Cwd: "/src/gr",
				Status: "blocked", Tags: map[string]string{"worktree": "fix"}, Windows: []int{3}},
			{ID: "b2", Name: "old", State: "shelved", Host: "server"},
		},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, %v\nwant %#v", got, err, want)
	}
	wantSent := map[string]any{"windows": []any{map[string]any{
		"id": 3.0, "pid": 99.0, "cmdline": []any{"mux", "attach"}, "foreground": []any{[]any{"vim", "x"}},
		"user_vars": map[string]any{"k": "v"}, "cwd": "/src",
	}}}
	if got := sent(t, dir, "list"); !reflect.DeepEqual(got, wantSent) {
		t.Errorf("sent %v, want %v", got, wantSent)
	}
}

// A layout saved from windows that show the provider's sessions has what the provider says runs in them,
// by the windows it lists each with; a provider that does not list has nothing to say.
func TestProviderShows(t *testing.T) {
	dir, _ := fakeProvider(t, map[string]string{
		"capabilities": `{"capabilities": ["list"]}`,
		"list":         `{"sessions": [{"id": "a1", "name": "build", "command": "make", "windows": [1, 2]}, {"id": "b2", "name": "idle"}]}`,
	}, nil)
	tabs := []kitty.FullTab{{Windows: []kitty.FullWindow{{ID: 1, PID: 7}, {ID: 2}, {ID: 3, UserVars: map[string]string{"kspaces": "1"}}}}}
	build := ProviderSession{ID: "a1", Name: "build", Command: "make", Windows: []int{1, 2}}
	if got, want := providerShows([]string{"prov"}, tabs), map[int]ProviderSession{1: build, 2: build}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
	w := func(id, pid float64) map[string]any {
		return map[string]any{"id": id, "pid": pid, "cmdline": []any{}, "foreground": []any{}, "user_vars": map[string]any{}, "cwd": ""}
	}
	if got, want := sent(t, dir, "list"), map[string]any{"windows": []any{w(1, 7), w(2, 0)}}; !reflect.DeepEqual(got, want) {
		t.Errorf("sent %v, want %v", got, want)
	}
	fakeProvider(t, map[string]string{"capabilities": `{"capabilities": ["keep"]}`}, nil)
	if got := providerShows([]string{"prov"}, tabs); got != nil {
		t.Errorf("a provider that does not list: %v", got)
	}
}
