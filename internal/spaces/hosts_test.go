package spaces

import (
	"reflect"
	"testing"
)

// HostViews lists this machine first with everything the list has and its own detached sessions, then
// each host the config names, by name, with its own copies once it has loaded, its sessions in a space it
// has no copy of as a space of that name, and its detached sessions; one that has not loaded says why. A
// host the provider lists sessions on but the config does not name has no kspaces server, and is left out.
func TestHostViews(t *testing.T) {
	gr := Space{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Path: "/src/gr", Session: "gr"}}}
	def := Space{Name: "default", Worktrees: []Worktree{{Session: "default"}}}
	homelab := Space{Name: "homelab", Root: "/home/me/homelab", Worktrees: []Worktree{{Path: "/home/me/homelab", Session: "white:homelab", Host: "white"}}}
	sessions := []ProviderSession{
		{ID: "a", Name: "a", Space: "gr"},
		{ID: "w:b", Name: "b", Space: "homelab", Host: "white"},
		{ID: "w:c", Name: "c", Space: "homelab-media", Host: "white"},
		{ID: "w:d", Name: "d", Space: "homelab-media", Host: "white"},
		{ID: "d:e", Name: "e", Space: "gr", Host: "dune"},
	}
	detached := map[string]Detached{
		"gr":            {Hosts: []HostSessions{{Sessions: []ProviderSession{sessions[0]}}, {Host: "dune", Sessions: []ProviderSession{sessions[4]}}}},
		"homelab":       {Hosts: []HostSessions{{Host: "white", Sessions: []ProviderSession{sessions[1]}}}},
		"homelab-media": {Hosts: []HostSessions{{Host: "white", Sessions: sessions[2:4]}}},
	}
	remotes := map[string]RemoteView{
		"white": {Loaded: true, Spaces: []Space{homelab}},
		"nas":   {Err: "ssh: connect to host nas: Connection refused"},
	}
	got := HostViews([]Space{def}, []Space{gr}, sessions, detached, remotes)
	want := []HostView{
		{Others: []Space{def}, Spaces: []Space{gr}, Detached: map[string]Detached{"gr": {Hosts: []HostSessions{{Sessions: []ProviderSession{sessions[0]}}}}}},
		{Host: "nas", Detached: map[string]Detached{}, Err: "ssh: connect to host nas: Connection refused"},
		{Host: "white", Spaces: []Space{homelab},
			Others: []Space{{Name: "homelab-media", Worktrees: []Worktree{{Session: "white:homelab-media", Host: "white"}}}},
			Detached: map[string]Detached{
				"homelab":       {Hosts: []HostSessions{{Host: "white", Sessions: []ProviderSession{sessions[1]}}}},
				"homelab-media": {Hosts: []HostSessions{{Host: "white", Sessions: sessions[2:4]}}},
			}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("views:\n%#v\nwant\n%#v", got, want)
	}
}
