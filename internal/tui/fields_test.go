package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
)

var (
	clickSpace = spaces.Space{Name: "gr", Root: "/src/gr", Worktrees: []spaces.Worktree{{Path: "/src/gr", Branch: "main", Session: "gr"}}}
	clickWt    = spaces.Worktree{Path: "/src/gr/.worktrees/fix", Branch: "fix", Session: "gr@fix"}
	clickOpen  = kitty.State{Sessions: map[string]int{"gr@fix": 7}}
)

// A click on a run form's field, its label or its box, focuses it as tab would, and one in its text puts
// the cursor on the character clicked.
func TestRunFormClickField(t *testing.T) {
	f := newRunForm(testCommands(t), clickSpace, clickWt, clickOpen, "claude", map[string]string{"prompt": "fix it"}, "")
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	screen := func(command, prompt, keys string) []string {
		return []string{"", "  Run in gr@fix", "", command, prompt, "", "  Runs claude", "  Opens in a new tab in gr@fix", "", keys, ""}
	}
	want := screen("  command    < claude >  left/right change  enter list", "  Prompt     fix it",
		"   Run    Cancel esc   tab next field  left/right command  enter/space list")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("at first:\n%q\nwant\n%q", got, want)
	}
	clickOn(t, f, "Prompt")
	typeInto(f, "!")
	want = screen("  command    < claude >", "  Prompt     fix it!", "   Run enter    Cancel esc   tab next field")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("the label clicked:\n%q\nwant\n%q", got, want)
	}
	// On the i of it, then past the end of the text.
	clickInto(t, f, "fix it!", 4)
	typeInto(f, "X")
	clickInto(t, f, "fix Xit!", 20)
	typeInto(f, "Y")
	if got, want := formScreen(f), screen("  command    < claude >", "  Prompt     fix Xit!Y", "   Run enter    Cancel esc   tab next field"); !reflect.DeepEqual(got, want) {
		t.Errorf("the text clicked:\n%q\nwant\n%q", got, want)
	}
	// Tab goes on from the field clicked, round to the choice.
	f.Update(keyMsg("tab"))
	if got, want := formScreen(f), screen("  command    < claude >  left/right change  enter list", "  Prompt     fix Xit!Y",
		"   Run    Cancel esc   tab next field  left/right command  enter/space list"); !reflect.DeepEqual(got, want) {
		t.Errorf("tab after:\n%q\nwant\n%q", got, want)
	}
}

// A click on an open layout form's field focuses it, its cursor where the click is.
func TestLayoutFormClickField(t *testing.T) {
	f := newLayoutForm(testLayouts(t), clickSpace, clickWt, clickOpen, "agent", map[string]string{"prompt": "fix it"})
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	clickInto(t, f, "fix it", 3)
	typeInto(f, ",")
	want := []string{"", "  Open a layout in gr@fix", "", "  layout     < agent >", "  Prompt     fix, it", "",
		"  A tab of claude, review, a shell", "  Adds its tabs to gr@fix", "", "   Open enter    Cancel esc   tab next field", ""}
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("clicked:\n%q\nwant\n%q", got, want)
	}
}

// A click on any of the new worktree form's fields focuses it: the branch, the directory and a layout's
// input. The branch, too long for its box, may be scrolled, so a click in it puts the cursor at its end.
func TestNewWorktreeFormClickField(t *testing.T) {
	f := newForm(testLayouts(t), clickSpace)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	typeInto(f, "fix")
	clickOn(t, f, "directory")
	typeInto(f, "here")
	clickOn(t, f, "Prompt")
	typeInto(f, "go")
	screen := func(branch, dir, does string) []string {
		return []string{"", "  New worktree in gr", "", branch, dir, "  layout     < agent >", "  Prompt     go",
			"  command    < none >", "", does, "  Opens with the agent layout", "", "   Create enter    Cancel esc   tab next field", ""}
	}
	if got, want := formScreen(f), screen("  branch     fix", "  directory  here", "  Creates fix from main"); !reflect.DeepEqual(got, want) {
		t.Errorf("clicked:\n%q\nwant\n%q", got, want)
	}
	clickInto(t, f, "fix", 1)
	typeInto(f, "e")
	long := strings.Repeat("x", 60)
	typeInto(f, long)
	clickInto(t, f, "branch", labelWidth+2)
	typeInto(f, "Y")
	branch := "fe" + long + "ixY"
	if got, want := formScreen(f), screen("  branch     "+branch[len(branch)-54:], "  directory  here", "  Creates "+branch[:55]+"..."); !reflect.DeepEqual(got, want) {
		t.Errorf("long branch:\n%q\nwant\n%q", got, want)
	}
}

// A click on the save form's name or a run box focuses it.
func TestSaveLayoutFormClickField(t *testing.T) {
	f, _ := saveEnv(t, "", true)
	clickOn(t, f, "make watch {{x}}")
	typeInto(f, "!")
	clickOn(t, f, "name")
	typeInto(f, "w")
	want := []string{
		"", "  Save gr@fix as a layout", "",
		"  name       w",
		"  save       < this tab >",
		"  save to    < the global config >",
		"  tab 1, pane 1: make watch {x}",
		"  runs       < run >",
		"  run        !make watch {{x}}",
		"", "  Adds 1 tab to the global config", "",
		"   Save enter    Cancel esc   tab next field", "",
	}
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("clicked:\n%q\nwant\n%q", got, want)
	}
}

// A click on the add space form's path puts its cursor there.
func TestAddSpaceFormClickField(t *testing.T) {
	f := newAddSpaceForm(nil, "/src/gr")
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	clickInto(t, f, "/src/gr", 4)
	typeInto(f, "x")
	want := []string{"", "  Add a space", "", "  repository /srcx/gr", "", "  /srcx/gr is not a directory", "", "   Add    Cancel esc", ""}
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("clicked:\n%q\nwant\n%q", got, want)
	}
}
