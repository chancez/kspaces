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
	got := HostViews([]Space{def}, []Space{gr, docs}, sessions, detached)
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
