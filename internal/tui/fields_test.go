package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/gittest"
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
	want := screen("  command    < claude >  left/right change  space list", "  Prompt     fix it",
		"   Run enter    Cancel esc   tab next field  left/right command  space list")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("at first:\n%q\nwant\n%q", got, want)
	}
	clickOn(t, f, "Prompt")
	typeInto(f, "!")
	want = screen("  command    < claude >", "  Prompt     fix it!", "   Run enter    Cancel esc   tab next field  shift+enter newline")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("the label clicked:\n%q\nwant\n%q", got, want)
	}
	// On the i of it, then past the end of the text.
	clickInto(t, f, "fix it!", 4)
	typeInto(f, "X")
	clickInto(t, f, "fix Xit!", 20)
	typeInto(f, "Y")
	if got, want := formScreen(f), screen("  command    < claude >", "  Prompt     fix Xit!Y", "   Run enter    Cancel esc   tab next field  shift+enter newline"); !reflect.DeepEqual(got, want) {
		t.Errorf("the text clicked:\n%q\nwant\n%q", got, want)
	}
	// Tab goes on from the field clicked, round to the choice.
	f.Update(keyMsg("tab"))
	if got, want := formScreen(f), screen("  command    < claude >  left/right change  space list", "  Prompt     fix Xit!Y",
		"   Run enter    Cancel esc   tab next field  left/right command  space list"); !reflect.DeepEqual(got, want) {
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
		"  A tab of claude, review, a shell", "  Adds its tabs to gr@fix", "", "   Open enter    Cancel esc   tab next field  shift+enter newline", ""}
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
	screen := func(branch, dir, does, keys string) []string {
		return []string{"", "  New worktree in gr", "", branch, dir, "  layout     < agent >", "  Prompt     go",
			"  command    < none >", "", does, "  Opens with the agent layout", "", "   Create enter    Cancel esc   tab next field" + keys, ""}
	}
	if got, want := formScreen(f), screen("  branch     fix", "  directory  here", "  Creates fix from main", "  shift+enter newline"); !reflect.DeepEqual(got, want) {
		t.Errorf("clicked:\n%q\nwant\n%q", got, want)
	}
	clickInto(t, f, "fix", 1)
	typeInto(f, "e")
	long := strings.Repeat("x", 60)
	typeInto(f, long)
	clickInto(t, f, "branch", labelWidth+2)
	typeInto(f, "Y")
	branch := "fe" + long + "ixY"
	if got, want := formScreen(f), screen("  branch     "+branch[len(branch)-54:], "  directory  here", "  Creates "+branch[:57]+"\u2026", ""); !reflect.DeepEqual(got, want) {
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

var (
	shiftEnter = tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
	altEnter   = tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt}
	ctrlJ      = tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}
	upKey      = tea.KeyPressMsg{Code: tea.KeyUp}
	downKey    = tea.KeyPressMsg{Code: tea.KeyDown}
)

// A command's input takes newlines: shift+enter, alt+enter and ctrl+j each start a line, and the field grows
// to show them, moving what is below it down. enter still runs, the value one word of the command line,
// newlines and all.
func TestRunFormLines(t *testing.T) {
	gittest.Isolate(t)
	calls := fakeKittenLs(t, lsWith("gr@fix"))
	cfg := testCommands(t)
	cfg.Launch = []string{"launch"}
	f := newRunForm(cfg, clickSpace, clickWt, clickOpen, "claude", nil, "")
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	f.Update(keyMsg("tab"))
	typeInto(f, "fix")
	f.Update(shiftEnter)
	typeInto(f, "it")
	f.Update(altEnter)
	typeInto(f, "now")
	f.Update(ctrlJ)
	typeInto(f, "ok")
	want := []string{"", "  Run in gr@fix", "", "  command    < claude >",
		"  Prompt     fix", "             it", "             now", "             ok", "",
		"  Runs claude", "  Opens in a new tab in gr@fix", "", "   Run enter    Cancel esc   tab next field  shift+enter newline", ""}
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("four lines:\n%q\nwant\n%q", got, want)
	}
	// A click on a line puts the cursor there.
	clickInto(t, f, "now", 1)
	typeInto(f, "-")
	want[6] = "             n-ow"
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("clicked:\n%q\nwant\n%q", got, want)
	}

	_, cmd := f.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("enter ran nothing")
	}
	if _, quit := f.Update(cmd()); quit == nil || f.err != nil {
		t.Fatalf("the form stayed: %v", f.err)
	}
	if got, want := calls(), []string{
		"@", "action", "goto_session", "gr@fix.kitty-session", "--",
		"@", "action", "--match=id:7", "launch", "--type=tab", "--cwd=/src/gr/.worktrees/fix", "--add-to-session=gr@fix",
		"--env=KSPACES_SPACE=gr", "--env=KSPACES_ROOT=/src/gr", "--env=KSPACES_WORKTREE=/src/gr/.worktrees/fix",
		"--env=KSPACES_BRANCH=fix", "--env=KSPACES_BASE=main", "--env=KSPACES_SESSION=gr@fix", "--env=KSPACES_PR=",
		"'--env=KSPACES_INPUT_PROMPT=fix\nit\nn-ow\nok'", "--hold", "claude", "'fix\nit\nn-ow\nok'", "--",
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("kitten ran as\n%q\nwant\n%q", got, want)
	}
}

// A field shows six lines at most, then scrolls with the cursor. up and down move between its lines, and
// leave it only from the first line or the last.
func TestTextFieldScrolls(t *testing.T) {
	f := newLayoutForm(testLayouts(t), clickSpace, clickWt, clickOpen, "agent", nil)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	f.Update(keyMsg("tab"))
	for i := 1; i <= 8; i++ {
		if i > 1 {
			f.Update(shiftEnter)
		}
		typeInto(f, fmt.Sprintf("line %d", i))
	}
	// The lines from first, with focus in them or on the choice above.
	screen := func(first int, onChoice bool) []string {
		lines := []string{"", "  Open a layout in gr@fix", "", "  layout     < agent >"}
		keys := "   Open enter    Cancel esc   tab next field  shift+enter newline"
		if onChoice {
			lines[3] += "  left/right change  space list"
			keys = "   Open enter    Cancel esc   tab next field  left/right layout  space list"
		}
		for i := first; i < first+6; i++ {
			label := "             "
			if i == first {
				label = "  Prompt     "
			}
			lines = append(lines, fmt.Sprintf("%sline %d", label, i))
		}
		return append(lines, "", "  A tab of claude, review, a shell", "  Adds its tabs to gr@fix", "", keys, "")
	}
	if got, want := formScreen(f), screen(3, false); !reflect.DeepEqual(got, want) {
		t.Errorf("eight lines:\n%q\nwant\n%q", got, want)
	}
	for range 7 {
		f.Update(upKey)
	}
	if got, want := formScreen(f), screen(1, false); !reflect.DeepEqual(got, want) {
		t.Errorf("up to the first line:\n%q\nwant\n%q", got, want)
	}
	// Up from the first line goes to the field above, and down comes back to it.
	f.Update(upKey)
	if got, want := formScreen(f), screen(1, true); !reflect.DeepEqual(got, want) {
		t.Errorf("up from the first line:\n%q\nwant\n%q", got, want)
	}
	for range 8 {
		f.Update(downKey)
	}
	if got, want := formScreen(f), screen(3, false); !reflect.DeepEqual(got, want) {
		t.Errorf("down to the last line:\n%q\nwant\n%q", got, want)
	}
	// Down from the last line goes round to the choice.
	f.Update(downKey)
	if got, want := formScreen(f), screen(3, true); !reflect.DeepEqual(got, want) {
		t.Errorf("down from the last line:\n%q\nwant\n%q", got, want)
	}
}
