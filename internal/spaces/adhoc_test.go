package spaces

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/config"
)

// An ad hoc space is named as cm names a space, and is not default nor a configured space.
func TestCheckAdHocName(t *testing.T) {
	cfg := config.Config{Spaces: []string{"/src/gr"}}
	got := map[string]bool{}
	for _, name := range []string{"scratch", "incident-123", ".dotfiles", "a.b_c", "", ".", "..", "has space", "a/b", "default", "gr", string(make([]byte, 65))} {
		got[name] = CheckAdHocName(cfg, name) == nil
	}
	want := map[string]bool{"scratch": true, "incident-123": true, ".dotfiles": true, "a.b_c": true, "": false, ".": false, "..": false,
		"has space": false, "a/b": false, "default": false, "gr": false, string(make([]byte, 65)): false}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("valid %v, want %v", got, want)
	}
}

// A host keeps its ad hoc spaces in a file, by name, so a server started again has them; a second of one
// name is refused, and one removed is gone. Each load carries them.
func TestAdHocSpaces(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	if err := os.MkdirAll(filepath.Join(cfgDir, "kspaces"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "kspaces", "kspaces.toml"), []byte("spaces = []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "state")
	h := NewLocalHost("", dir)
	h.load = func() HostData { return HostData{Config: &config.Config{}} }
	for _, s := range []AdHocSpace{{Name: "scratch"}, {Name: "incident", Dir: "/tmp/incident"}} {
		if err := h.adHoc.add(s); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.adHoc.add(AdHocSpace{Name: "scratch"}); !errors.Is(err, ErrSpaceExists) {
		t.Errorf("a second scratch: %v", err)
	}
	again := NewLocalHost("", dir)
	again.load = h.load
	hd, _ := again.Load(true)
	if want := []AdHocSpace{{Name: "incident", Dir: "/tmp/incident"}, {Name: "scratch"}}; !reflect.DeepEqual(hd.AdHoc, want) {
		t.Errorf("kept %+v, want %+v", hd.AdHoc, want)
	}
	if err := again.RemoveSpace("scratch"); err != nil {
		t.Fatal(err)
	}
	hd, _ = again.Load(true)
	if want := []AdHocSpace{{Name: "incident", Dir: "/tmp/incident"}}; !reflect.DeepEqual(hd.AdHoc, want) {
		t.Errorf("after removing scratch %+v, want %+v", hd.AdHoc, want)
	}
}
