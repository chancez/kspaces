package config

import (
	"reflect"
	"testing"
)

// Hosts are defined a file each, and an overlay takes the place of one called the same. ssh goes to the
// host's name, and runs kspaces from PATH, unless it says otherwise.
func TestHosts(t *testing.T) {
	configDir(t, map[string]string{
		"kspaces.toml":       "spaces = []\n\n[hosts.white]\n\n[hosts.nas]\nssh = \"me@nas.lan\"\n",
		"conf.d/desk.toml":   "[hosts.desk]\nkspaces = \"~/.local/bin/kspaces\"\n",
		"kspaces.local.toml": "[hosts.nas]\nssh = \"nas\"\n",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]HostSettings{"white": {}, "nas": {SSH: "nas"}, "desk": {Kspaces: "~/.local/bin/kspaces"}}
	if !reflect.DeepEqual(cfg.Hosts, want) {
		t.Errorf("hosts %+v, want %+v", cfg.Hosts, want)
	}
	got := [][2]string{}
	for _, name := range []string{"white", "nas", "desk"} {
		h := cfg.Hosts[name]
		got = append(got, [2]string{h.Destination(name), h.Command()})
	}
	if want := [][2]string{{"white", "kspaces"}, {"nas", "kspaces"}, {"desk", "~/.local/bin/kspaces"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("reached as %q, want %q", got, want)
	}
}

// A host defined in two files that are not overlays is an error naming both.
func TestHostsTwice(t *testing.T) {
	dir := configDir(t, map[string]string{
		"kspaces.toml":     "spaces = []\n\n[hosts.white]\n",
		"conf.d/work.toml": "[hosts.white]\nssh = \"white.lan\"\n",
	})
	_, err := Load()
	if want := dir + ": hosts: white is defined both in kspaces.toml and in conf.d/work.toml; define it in one"; err == nil || err.Error() != want {
		t.Errorf("Load() = %v, want %s", err, want)
	}
}
