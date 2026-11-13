package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"
)

// replyScreen is a screen of twelve lines, the second red, the last a prompt, and blank rows under it.
const replyScreen = "line 1\n\x1b[31mred 2\x1b[m\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\n" +
	"a line 11 too long to fit in the form, so it is cut where the form ends, here\n> \n\n\n"

// newTestReply is the form replying to window 7 of gr@fix, 80 by 16, its screen fetched.
func newTestReply(t *testing.T) *replyForm {
	t.Helper()
	f := newReplyForm(7, "gr@fix", "claude")
	var asked []int
	f.getText = func(w int) (string, error) { asked = append(asked, w); return replyScreen, nil }
	f.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	f.Update(f.Init()())
	if want := []int{7}; !reflect.DeepEqual(asked, want) {
		t.Fatalf("fetched the screens of %v, want %v", asked, want)
	}
	return f
}

// The form shows the window's last lines that fit above the field, cut to the form and with their colors,
// then the field and its keys; send without enter is there only with something to send.
func TestReplyForm(t *testing.T) {
	f := newTestReply(t)
	rule := "  " + strings.Repeat("\u2500", 76)
	screen := func(field ...string) []string {
		lines := []string{"", "  Reply to gr@fix  claude", "",
			"  line 5", "  line 6", "  line 7", "  line 8", "  line 9", "  line 10",
			"  a line 11 too long to fit in the form, so it is cut where the form ends, her", "  >", rule}
		return append(append(lines, field...), "")
	}
	want := screen("  reply      Enter alone sends Enter", "",
		"   Send enter    Cancel esc   shift+enter newline")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("empty:\n%q\nwant\n%q", got, want)
	}
	typeInto(f, "yes")
	f.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	typeInto(f, "and")
	want = []string{"", "  Reply to gr@fix  claude", "",
		"  line 6", "  line 7", "  line 8", "  line 9", "  line 10",
		"  a line 11 too long to fit in the form, so it is cut where the form ends, her", "  >", rule,
		"  reply      yes", "             and", "",
		"   Send enter    Send without enter ctrl+s    Cancel esc   shift+enter newline", ""}
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("two lines typed:\n%q\nwant\n%q", got, want)
	}

	// The red line keeps its color, ended where the line is.
	f.Update(tea.WindowSizeMsg{Width: 80, Height: 26})
	var red string
	for _, l := range strings.Split(f.render(), "\n") {
		if strings.Contains(l, "red 2") {
			red = l
		}
	}
	if want := "  \x1b[31mred 2\x1b[m\x1b[m" + strings.Repeat(" ", 73); red != want {
		t.Errorf("the red line: %q, want %q", red, want)
	}

	// A screen that could not be fetched shows none.
	f = newReplyForm(7, "", "")
	f.getText = func(int) (string, error) { return "", fmt.Errorf("no window") }
	f.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	f.Update(f.Init()())
	want = []string{"", "  Reply to window 7", "", "  reply      Enter alone sends Enter", "",
		"   Send enter    Cancel esc   shift+enter newline", ""}
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("no screen:\n%q\nwant\n%q", got, want)
	}
}

// enter sends the text and then Enter, an empty field just Enter, and ctrl+s the text alone; a newline
// goes as typed and a backslash as one. Each closes the form.
func TestReplySends(t *testing.T) {
	calls := fakeKittenLs(t, "")
	for _, c := range []struct {
		typed []tea.KeyPressMsg
		key   tea.KeyPressMsg
	}{
		{keys("yes"), keyMsg("enter")},
		{nil, keyMsg("enter")},
		{keys("1"), tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}},
		{append(append(keys("a"), tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}), keys(`b\n`)...), keyMsg("enter")},
	} {
		f := newTestReply(t)
		for _, k := range c.typed {
			f.Update(k)
		}
		_, cmd := f.Update(c.key)
		if cmd == nil {
			t.Fatalf("%v ran nothing", c.key)
		}
		if got, want := formScreen(f)[12:], []string{"  Sending...", "", "   Cancel esc   shift+enter newline", ""}; !reflect.DeepEqual(got, want) {
			t.Errorf("sending:\n%q\nwant\n%q", got, want)
		}
		if _, quit := f.Update(cmd()); quit == nil || f.err != nil {
			t.Fatalf("the form stayed: %v", f.err)
		}
	}
	send := func(text string) []string { return []string{"@", "send-text", "--match", "id:7", "--", text, "--"} }
	var want []string
	for _, text := range []string{`yes\r`, `\r`, "1", "a\n" + `b\\n\r`} {
		want = append(want, send(text)...)
	}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("kitten ran as\n%q\nwant\n%q", got, want)
	}

	// ctrl+s has nothing to send in an empty field.
	f := newTestReply(t)
	f.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if f.busy {
		t.Error("ctrl+s sent an empty field")
	}
}

// keys are the key presses typing s.
func keys(s string) []tea.KeyPressMsg {
	var out []tea.KeyPressMsg
	for _, r := range s {
		out = append(out, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return out
}

// Reply is on a status entry's menu, which m opens, and t its key; either opens the reply form for the
// entry's window in a popup.
func TestReplyKey(t *testing.T) {
	e := newSidebarEnv(t)
	launched := fakeKitten(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"@", "launch", "--type=overlay", "--var=kspaces_form=1", "--next-to", "id:11", exe, "reply", "21"}
	e.m.Update(tea.FocusMsg{})
	i := e.entry(21)
	e.m.cursor = i
	e.m.render()
	e.press("m")
	_, y := e.zone(fmt.Sprintf(entryZone, i))
	if got, want := e.screen()[y+1:y+5], []string{menuRule(""), " Open", " Reply", menuRule("")}; !reflect.DeepEqual(got, want) {
		t.Errorf("menu:\n%q\nwant\n%q", got, want)
	}
	e.press("j")
	if cmd := e.press("enter"); cmd == nil {
		t.Fatal("Reply ran nothing")
	} else if msg := cmd(); msg != nil {
		t.Errorf("Reply said %v", msg)
	}
	if got := launched(); !reflect.DeepEqual(got, want) {
		t.Errorf("Reply launched %q, want %q", got, want)
	}
	// fakeKitten's log, in the directory PATH is, so t is seen to launch it again.
	if err := os.Remove(filepath.Join(os.Getenv("PATH"), "args")); err != nil {
		t.Fatal(err)
	}
	cmd, ok := runOn(e.m, keyMsg("t"), e.m.chain()...)
	if !ok || cmd == nil {
		t.Fatal("t ran nothing")
	}
	if msg := cmd(); msg != nil {
		t.Errorf("t said %v", msg)
	}
	if got := launched(); !reflect.DeepEqual(got, want) {
		t.Errorf("t launched %q, want %q", got, want)
	}
}

// A right click on a status entry opens its menu under the mouse, Open and Reply, as on a spaces row; on
// an entry's hover popup, the menu of the entry it shows, not of the one it covers.
func TestStatusRightClick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newSidebarEnv(t)
		rule := strings.Repeat("\u2500", 30)
		top := []string{
			spacesTitle, "", "  \uf120  default", "", "\u25bc \uf401  gr main", here + "\uf071  dev pr/me/dev", "  \ue725  fix fix", "",
			"  \uf401  kitty master", "", "\u25b6 \uf110  docs main", "", "  \uf00d  broken", "", "", "", "", "", "", "", "", "",
			rule, "STATUS", "Blocked 1", "\uf071  dev                   +1 5m", "   claude \u00b7 approve", "", "Failed 1", "\ue654  scratch",
		}
		menu := []string{rule, " Open", " Reply", rule}
		screen := func(lines ...[]string) []string {
			var out []string
			for _, l := range lines {
				out = append(out, l...)
			}
			return out
		}
		e.click(fmt.Sprintf(entryZone, e.entry(21)), tea.MouseRight)
		want := screen(top, menu, []string{"   make \u00b7 \u2588\u2588\u2588\u2588\u2588\u2588\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591 40%", "", "Idle 1", "\uf10c  default", "   claude", ""})
		if got := e.screen(); !reflect.DeepEqual(got, want) {
			t.Errorf("on the failed entry:\n%q\nwant\n%q", got, want)
		}
		e.press("esc")

		// The blocked entry's popup covers the failed one.
		e.hover(fmt.Sprintf(entryZone, e.entry(20)))
		_, y := e.zone(popupZone)
		e.clickAt(4, y+5, tea.MouseRight)
		want = screen(top, []string{"   exit 1"}, menu, []string{"", "Idle 1", "\uf10c  default", "   claude", ""})
		if got := e.screen(); !reflect.DeepEqual(got, want) || e.m.cursor != e.entry(20) {
			t.Errorf("on the popup, cursor %d, want %d:\n%q\nwant\n%q", e.m.cursor, e.entry(20), got, want)
		}
	})
}
