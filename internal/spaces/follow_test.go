package spaces

import (
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// A session's space changing is what moves its windows, not its space differing from theirs: the first sight
// of a session only records it, so starting the sidebar rearranges nothing, and one that had no windows is
// recorded when it gains them.
func TestSpaceWatchChanged(t *testing.T) {
	var w spaceWatch
	shown := func(id, space string) ProviderSession {
		return ProviderSession{ID: id, Space: space, Windows: []int{1}}
	}
	first := &ProviderState{Sessions: []ProviderSession{shown("a", ""), shown("b", "cm"), {ID: "c", Space: "cm"}}}
	if got := w.changed(first); len(got) != 0 {
		t.Errorf("first poll changed %v, want nothing", got)
	}
	second := &ProviderState{Sessions: []ProviderSession{shown("a", "homelab"), shown("b", "cm"), shown("c", "infra")}}
	if got, want := w.changed(second), []ProviderSession{shown("a", "homelab")}; !reflect.DeepEqual(got, want) {
		t.Errorf("second poll changed %v, want %v", got, want)
	}
	if got := w.changed(second); len(got) != 0 {
		t.Errorf("third poll changed %v, want nothing once recorded", got)
	}
}

// The windows of a moved session move into a tab of its new space's session, each by the sidebar of its own
// OS window, and the keyboard goes along only from the window that had it. A window already in the space, and
// one of kspaces' own, stay.
func TestPlanMoves(t *testing.T) {
	data := []kitty.OSWindow{
		{IsFocused: true, Tabs: []kitty.Tab{
			{IsActive: true, Windows: []kitty.Window{
				{ID: 1, UserVars: map[string]string{"kspaces_dock": "1"}},
				{ID: 2, SessionName: "default", IsActive: true},
			}},
			{Windows: []kitty.Window{{ID: 3, SessionName: "cm@fix", IsActive: true}}},
			{Windows: []kitty.Window{{ID: 4, SessionName: "homelab", IsActive: true}}},
		}},
		{Tabs: []kitty.Tab{{IsActive: true, Windows: []kitty.Window{{ID: 9, SessionName: "default", IsActive: true}}}}},
	}
	homelab := Space{Name: "homelab", Worktrees: []Worktree{{Session: "homelab"}}}
	moved := ProviderSession{ID: "white:@aaaa", Space: "homelab", Windows: []int{2, 3, 4, 9}}
	own := ProviderSession{ID: "@bbbb", Space: "notes", Windows: []int{1}}

	got := planMoves([]ProviderSession{moved, own}, []Space{homelab}, kitty.State{}, data, 1)
	want := []spaceMove{
		{Window: 2, Session: moved, Target: Worktree{Session: "homelab"}, Focused: true},
		{Window: 3, Session: moved, Target: Worktree{Session: "homelab"}},
		// 4 is in homelab already, 9 is another OS window's sidebar's, and 1 is the sidebar itself.
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("planMoves() = %#v\nwant %#v", got, want)
	}

	// A space no configured space is, a topic space, is a session of its own name.
	topic := ProviderSession{ID: "@cccc", Space: "scratch", Windows: []int{3}}
	if got, want := planMoves([]ProviderSession{topic}, nil, kitty.State{}, data, 1),
		[]spaceMove{{Window: 3, Session: topic, Target: Worktree{Session: "scratch"}}}; !reflect.DeepEqual(got, want) {
		t.Errorf("planMoves(topic) = %#v\nwant %#v", got, want)
	}
}

// A window moving without the keyboard opens behind: a tab with --keep-focus, in the space's session and by
// its window when it has one, and no goto_session, which would switch to it.
func TestBehindLaunch(t *testing.T) {
	cfg := config.Config{Launch: []string{"launch"}}
	got := behindLaunch(cfg, Worktree{Session: "cm@fix", Path: "/src/cm/.worktrees/fix"}, 7, []string{"cm", "attach", "white:@aaaa"}, "")
	want := []string{
		"--match=id:7", "launch", "--type=tab", "--keep-focus", kitty.Quote("--cwd=/src/cm/.worktrees/fix"),
		"--add-to-session=cm@fix", kitty.Quote("--var=kspaces_space=cm"), kitty.Quote("--var=kspaces_worktree=fix"),
		kitty.Quote("cm"), kitty.Quote("attach"), kitty.Quote("white:@aaaa"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("behindLaunch() = %q\nwant %q", got, want)
	}
	// With no window of its session to go by, the tab is the session's first.
	if got := behindLaunch(cfg, Worktree{Session: "scratch"}, 0, []string{"x"}, "/tmp"); got[0] != "launch" {
		t.Errorf("behindLaunch() with no anchor = %q, want it to start at launch", got)
	}
}
