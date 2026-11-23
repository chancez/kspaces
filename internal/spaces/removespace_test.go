package spaces

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gittest"
)

// Removing a configured space ends the provider's sessions in it, closes its kitty sessions and forgets
// their files, takes it out of config.toml, its repository kept, and has the provider remove its space.
func TestRemoveConfiguredSpace(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	name := filepath.Base(r.Dir)
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	cfgPath := filepath.Join(home, "kspaces", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("spaces = [\n  \""+r.Dir+"\",\n  \"/src/kitty\",\n]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sessions := t.TempDir()
	if err := os.WriteFile(filepath.Join(sessions, name+"@old.kitty-session"), []byte("launch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, provider := fakeProvider(t, map[string]string{
		"capabilities": `{"capabilities": ["list", "kill", "remove", "remove-space"]}`,
		"list": `{"spaces": ["` + name + `"], "sessions": [{"id": "a", "space": "` + name + `", "state": "running", "windows": [10]},
			{"id": "b", "space": "` + name + `", "state": "shelved"}, {"id": "c", "space": "other", "state": "running"}]}`,
	}, nil)
	kitten := fakeKittenLs(t, dir, `[{"tabs": [{"windows": [{"id": 10, "session_name": "`+name+`"}]}]}]`)
	cfg := config.Config{Spaces: []string{r.Dir, "/src/kitty"}, Provider: []string{"prov"}, SessionsDir: sessions}
	plan, err := PlanSpaceRemoval(cfg, name)
	if err != nil {
		t.Fatal(err)
	}
	want := SpaceRemoval{Name: name, Root: r.Dir, Sessions: []string{name, name + "@old"},
		Provider: []ProviderSession{{ID: "a", Space: name, State: "running", Windows: []int{10}}, {ID: "b", Space: name, State: "shelved"}},
		Caps:     Capabilities{CapList: true, CapKill: true, CapRemove: true, CapRemoveSpace: true},
		Fates:    []Fate{FateEnd, FateLeave}}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("plan %#v\nwant %#v", plan, want)
	}
	provider()
	kitten()
	if err := RemoveSpace(cfg, name, FateEnd); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(cfgPath)
	if got, want := string(data), "spaces = [\n  \"/src/kitty\",\n]\n"; got != want {
		t.Errorf("config.toml %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(sessions, name+"@old.kitty-session")); !os.IsNotExist(err) {
		t.Errorf("the saved session is still there: %v", err)
	}
	if _, err := os.Stat(r.Dir); err != nil {
		t.Errorf("the repository went: %v", err)
	}
	if got, want := sent(t, dir, "remove-space"), map[string]any{"space": name}; !reflect.DeepEqual(got, want) {
		t.Errorf("remove-space sent %v, want %v", got, want)
	}
}

// A topic space goes only with its sessions ended, as the provider removes an empty space alone; default
// is never one to remove.
func TestRemoveTopicSpace(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir, _ := fakeProvider(t, map[string]string{
		"capabilities": `{"capabilities": ["list", "remove", "remove-space"]}`,
		"list":         `{"spaces": ["notes", "default"], "sessions": [{"id": "n", "space": "notes", "state": "exited"}]}`,
	}, map[string]string{})
	fakeKittenLs(t, dir, `[]`)
	cfg := config.Config{Provider: []string{"prov"}}
	plan, err := PlanSpaceRemoval(cfg, "notes")
	if want := []Fate{FateEnd}; err != nil || !reflect.DeepEqual(plan.Fates, want) {
		t.Errorf("fates %v, %v, want %v", plan.Fates, err, want)
	}
	if err := RemoveSpace(cfg, "notes", FateShelve); err == nil {
		t.Error("shelving a topic space's sessions removed it")
	}
	if err := RemoveSpace(cfg, "notes", FateEnd); err != nil {
		t.Fatal(err)
	}
	if got, want := []any{sent(t, dir, "remove"), sent(t, dir, "remove-space")}, []any{map[string]any{"session": "n"}, map[string]any{"space": "notes"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("sent %v, want %v", got, want)
	}
	if _, err := PlanSpaceRemoval(cfg, "default"); err == nil {
		t.Error("default could be removed")
	}
}
