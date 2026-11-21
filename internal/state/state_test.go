package state

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLegacy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_STATE_HOME", home)
	dir := filepath.Join(home, "kspaces")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"sort":   "status\n",
		"folded": "/src/gr\ngroup:work\n",
		"hidden": "art\n",
		"order":  `{"/src/gr": ["c", "a"]}`,
		"split":  "7\n",
		"trust":  "abc  /src/gr/.kspaces.toml\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want := State{
		Sidebar: Sidebar{
			Sort:       "status",
			Folded:     []string{"/src/gr", "group:work"},
			Hidden:     []string{"art"},
			Order:      map[string][]string{"/src/gr": {"c", "a"}},
			StatusRows: 7,
		},
		Trust: map[string]string{"/src/gr/.kspaces.toml": "abc"},
	}
	if got := Load(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}

	// The first write carries the legacy state over, and from then on the old files are not read.
	if err := Update(func(s *State) { s.Sidebar.Sort = "" }); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hidden"), []byte("other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	want.Version, want.Sidebar.Sort = Version, ""
	if got := Load(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() after Update = %#v, want %#v", got, want)
	}
}

func TestUpdate(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if got := Load(); !reflect.DeepEqual(got, State{}) {
		t.Fatalf("Load() of nothing = %#v", got)
	}
	if err := Update(func(s *State) { s.Trust = map[string]string{"/a": "1"} }); err != nil {
		t.Fatal(err)
	}
	if err := Update(func(s *State) { s.Sidebar.StatusRows = 3 }); err != nil {
		t.Fatal(err)
	}
	want := State{Version: Version, Sidebar: Sidebar{StatusRows: 3}, Trust: map[string]string{"/a": "1"}}
	if got := Load(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestUpdateBroken(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Load(); !reflect.DeepEqual(got, State{}) {
		t.Fatalf("Load() of a broken file = %#v", got)
	}
	if err := Update(func(s *State) { s.Sidebar.Sort = "status" }); err == nil {
		t.Fatal("Update wrote over a broken file")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "{not json" {
		t.Fatalf("broken file is now %q, %v", data, err)
	}
}
