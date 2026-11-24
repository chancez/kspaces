package tui

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// A click on a suspended session's row opens the resume form rather than the session; one with windows
// opens as ever.
func TestOpenSuspendedAsks(t *testing.T) {
	e := newSidebarEnv(t)
	calls := fakeKittenCalls(t)
	snap := fixture(time.Now())
	snap.Saved = map[string]bool{"gr@fix": true, "gr@dev": true}
	snap.Suspended = map[string]bool{"gr@fix": true, "gr@dev": true}
	e.m.Update(snap)
	e.m.render()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	open := func(session string) [][]string {
		e.t.Helper()
		cmd := e.click(fmt.Sprintf(entryZone, e.row(session)), tea.MouseLeft)
		if cmd == nil {
			t.Fatalf("a click on %s ran nothing", session)
		}
		before := len(calls())
		if msg := cmd(); msg != nil {
			if _, ok := msg.(failedMsg); ok {
				t.Fatalf("opening %s said %v", session, msg)
			}
		}
		return calls()[before:]
	}
	want := [][]string{{"@", "launch", "--type=overlay", "--var=kspaces_form=1", "--next-to", "id:11", exe, "resume", "gr@fix"}}
	if got := open("gr@fix"); !reflect.DeepEqual(got, want) {
		t.Errorf("suspended ran\n%q\nwant\n%q", got, want)
	}
	want = [][]string{{"@", "focus-window", "--match", "id:11"}}
	if got := open("gr@dev"); !reflect.DeepEqual(got, want) {
		t.Errorf("suspended with windows ran\n%q\nwant\n%q", got, want)
	}
}

// The resume form says what resuming does; n leaves without opening, and Resume opens the session.
func TestResumeSessionForm(t *testing.T) {
	wt := spaces.Worktree{Path: "/src/gr/.worktrees/fix", Branch: "fix", Session: "gr@fix"}
	var opened []spaces.Worktree
	f := &resumeSessionForm{th: look.New(true), wt: wt,
		open: func(_ config.Config, _ kitty.State, wt spaces.Worktree, _ spaces.OpenOptions) error {
			opened = append(opened, wt)
			return nil
		}}
	f.Update(tea.WindowSizeMsg{Width: 300, Height: 20})
	var screen []string
	for _, l := range strings.Split(ansi.Strip(f.render()), "\n") {
		screen = append(screen, strings.TrimRight(l, " "))
	}
	want := []string{
		"",
		"  Resume session gr@fix",
		"",
		"  It is suspended. Resuming restores its windows and what was kept running in them.",
		"",
		"   Resume y    Cancel n",
		"",
	}
	if !reflect.DeepEqual(screen, want) {
		t.Errorf("form:\n%q\nwant\n%q", screen, want)
	}
	if _, cmd := f.Update(tea.KeyPressMsg{Code: 'n', Text: "n"}); cmd == nil || f.busy || opened != nil {
		t.Errorf("n: quit %v, busy %v, opened %v", cmd != nil, f.busy, opened)
	}
	f.View()
	cmd := clickOn(t, f, "Resume y")
	if cmd == nil || !f.busy {
		t.Fatalf("Resume: cmd %v, busy %v", cmd != nil, f.busy)
	}
	if _, cmd := f.Update(cmd()); cmd == nil || f.err != nil {
		t.Fatalf("after resuming: quit %v, err %v", cmd != nil, f.err)
	}
	if want := []spaces.Worktree{wt}; !reflect.DeepEqual(opened, want) {
		t.Errorf("opened %v, want %v", opened, want)
	}
}
