package spaces

import (
	"reflect"
	"testing"
)

// HostViews lists this machine first with everything the spaces tab has, then each other host by name
// with the spaces and sessions in no space it has a session in: a configured space keeps its main
// worktree and the linked ones its sessions there are tagged with, and a tag no worktree here has is a
// worktree of that host's; a space it has that is neither is a space of its own. Each has its own
// detached sessions.
func TestHostViews(t *testing.T) {
	gr := Space{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{
		{Path: "/src/gr", Session: "gr"},
		{Path: "/src/gr/.worktrees/dev", Session: "gr@dev"},
		{Path: "/src/gr/.worktrees/fix", Session: "gr@fix"},
	}, Missing: []Worktree{{Session: "gr@old"}}}
	docs := Space{Name: "docs", Root: "/src/docs", Worktrees: []Worktree{{Path: "/src/docs", Session: "docs"}}}
	def := Space{Name: "default", Worktrees: []Worktree{{Session: "default"}}}
	sessions := []ProviderSession{
		{ID: "a", Name: "a", Space: "gr"},
		{ID: "s:b", Name: "b", Space: "gr", Host: "server", Tags: map[string]string{"worktree": "fix"}},
		{ID: "s:c", Name: "c", Space: "gr", Host: "server", Tags: map[string]string{"worktree": "far"}, Windows: []int{4}},
		{ID: "s:d", Name: "d", Host: "server"},
		{ID: "s:e", Name: "e", Space: "notes", Host: "server"},
		{ID: "l:f", Name: "f", Space: "docs", Host: "laptop"},
	}
	detached := map[string]Detached{
		"gr":      {Hosts: []HostSessions{{Sessions: []ProviderSession{sessions[0]}}, {Host: "server", Sessions: []ProviderSession{sessions[1]}}}},
		"default": {Hosts: []HostSessions{{Host: "server", Sessions: []ProviderSession{sessions[3]}}}},
		"notes":   {Hosts: []HostSessions{{Host: "server", Sessions: []ProviderSession{sessions[4]}}}},
		"docs":    {Hosts: []HostSessions{{Host: "laptop", Sessions: []ProviderSession{sessions[5]}}}},
	}
	got := HostViews([]Space{def}, []Space{gr, docs}, sessions, detached, nil)
	want := []HostView{
		{Others: []Space{def}, Spaces: []Space{gr, docs}, Detached: map[string]Detached{
			"gr": {Hosts: []HostSessions{{Sessions: []ProviderSession{sessions[0]}}}},
		}},
		{Host: "laptop", Spaces: []Space{{Name: "docs", Root: "/src/docs", Worktrees: []Worktree{{Path: "/src/docs", Session: "docs"}}}}, Detached: map[string]Detached{
			"docs": {Hosts: []HostSessions{{Host: "laptop", Sessions: []ProviderSession{sessions[5]}}}},
		}},
		{Host: "server",
			Spaces: []Space{{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{
				{Path: "/src/gr", Session: "gr"},
				{Path: "/src/gr/.worktrees/fix", Session: "gr@fix"},
				{Session: "gr@far", Host: "server"},
			}}},
			Others: []Space{def, {Name: "notes", Worktrees: []Worktree{{Session: "notes"}}}},
			Detached: map[string]Detached{
				"gr":      {Hosts: []HostSessions{{Host: "server", Sessions: []ProviderSession{sessions[1]}}}},
				"default": {Hosts: []HostSessions{{Host: "server", Sessions: []ProviderSession{sessions[3]}}}},
				"notes":   {Hosts: []HostSessions{{Host: "server", Sessions: []ProviderSession{sessions[4]}}}},
			}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("views:\n%#v\nwant\n%#v", got, want)
	}
}

// A host the config names that has loaded lists its own copies of its spaces, in place of what its
// sessions say, with the sessions in no space it has; one that has not loaded is listed as its sessions
// say, with why it could not.
func TestHostViewsRemote(t *testing.T) {
	gr := Space{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Path: "/src/gr", Session: "gr"}}}
	def := Space{Name: "default", Worktrees: []Worktree{{Session: "default"}}}
	homelab := Space{Name: "homelab", Root: "/home/me/homelab", Worktrees: []Worktree{{Path: "/home/me/homelab", Session: "white:homelab", Host: "white"}}}
	sessions := []ProviderSession{
		{ID: "w:a", Name: "a", Space: "homelab", Host: "white"},
		{ID: "w:b", Name: "b", Host: "white"},
		{ID: "n:c", Name: "c", Space: "gr", Host: "nas"},
	}
	remotes := map[string]RemoteView{
		"white": {Loaded: true, Spaces: []Space{homelab}},
		"nas":   {Err: "ssh: connect to host nas: Connection refused"},
		"desk":  {Err: "ssh: connect to host desk: Operation timed out"},
	}
	got := HostViews([]Space{def}, []Space{gr}, sessions, nil, remotes)
	want := []HostView{
		{Others: []Space{def}, Spaces: []Space{gr}, Detached: map[string]Detached{}},
		{Host: "desk", Detached: map[string]Detached{}, Err: "ssh: connect to host desk: Operation timed out"},
		{Host: "nas", Spaces: []Space{{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Path: "/src/gr", Session: "gr"}}}}, Detached: map[string]Detached{}, Err: "ssh: connect to host nas: Connection refused"},
		{Host: "white", Spaces: []Space{homelab}, Others: []Space{def}, Detached: map[string]Detached{}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("views:\n%#v\nwant\n%#v", got, want)
	}
}
