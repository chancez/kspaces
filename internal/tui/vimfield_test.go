package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// With vim on, a field starts in insert mode, its mode at its right. Esc goes to normal mode rather than
// leaving the form, where letters are vim's commands, not typing, and the arrows still move the cursor.
// Esc in normal mode leaves the form.
func TestFieldVim(t *testing.T) {
	f := newLayoutForm(testLayouts(t), clickSpace, clickWt, clickOpen, "agent", nil)
	f.theme().SetVim(true)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	f.Update(keyMsg("tab"))
	typeInto(f, "fix it")
	prompt := func() string { return formScreen(f)[4] }
	// The prompt's row, its mode at the right of the 70 cells but for the form's padding.
	row := func(text, mode string) string {
		line := "  Prompt     " + text
		return line + strings.Repeat(" ", 68-len(line)-len(mode)) + mode
	}
	if got, want := prompt(), row("fix it", "INSERT"); got != want {
		t.Errorf("typed:\n%q\nwant\n%q", got, want)
	}
	if _, cmd := f.Update(keyMsg("esc")); quits(cmd) {
		t.Fatal("esc in insert mode left the form")
	}
	// 0 to the start, x deletes the f, the right arrow passes the i, and i inserts before the x.
	typeInto(f, "0x")
	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	typeInto(f, "i")
	typeInto(f, "!")
	if got, want := prompt(), row("i!x it", "INSERT"); got != want {
		t.Errorf("edited:\n%q\nwant\n%q", got, want)
	}
	f.Update(keyMsg("esc"))
	if got, want := prompt(), row("i!x it", "NORMAL"); got != want {
		t.Errorf("normal:\n%q\nwant\n%q", got, want)
	}
	if _, cmd := f.Update(keyMsg("esc")); !quits(cmd) {
		t.Error("esc in normal mode kept the form open")
	}
}

// Without vim, esc leaves the form at once and no mode shows.
func TestFieldNoVim(t *testing.T) {
	f := newLayoutForm(testLayouts(t), clickSpace, clickWt, clickOpen, "agent", nil)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	f.Update(keyMsg("tab"))
	typeInto(f, "fix it")
	if got, want := formScreen(f)[4], "  Prompt     fix it"; !reflect.DeepEqual(got, want) {
		t.Errorf("typed:\n%q\nwant\n%q", got, want)
	}
	if _, cmd := f.Update(keyMsg("esc")); !quits(cmd) {
		t.Error("esc kept the form open")
	}
}

// A line field edits with vim as a text field does, in a form whose only field it is.
func TestLineFieldVim(t *testing.T) {
	f := newAddSpaceForm(nil, nil, "/src/gr", "")
	f.theme().SetVim(true)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	if _, cmd := f.Update(keyMsg("esc")); quits(cmd) {
		t.Fatal("esc in insert mode left the form")
	}
	typeInto(f, "0xx")
	if got, want := f.path.Value(), "rc/gr"; got != want {
		t.Errorf("value %q, want %q", got, want)
	}
	if _, cmd := f.Update(keyMsg("esc")); !quits(cmd) {
		t.Error("esc in normal mode kept the form open")
	}
}

// With vim the terminal's cursor is the field's, where it is in the text: a steady bar in insert mode and
// a steady block in normal mode. A panel draws the form a row higher, in its border, and the list of keys
// hides it. It has no color, keeping the terminal's. Without vim the field draws its own.
func TestFieldVimCursor(t *testing.T) {
	cursor := func(x, y int, shape tea.CursorShape) *tea.Cursor {
		c := tea.NewCursor(x, y)
		c.Shape, c.Blink = shape, false
		return c
	}
	f := newLayoutForm(testLayouts(t), clickSpace, clickWt, clickOpen, "agent", nil)
	f.theme().SetVim(true)
	p, _ := stubPopup(f, 11, false)
	p.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	p.Update(keyMsg("tab"))
	typeInto(p, "fix it")
	// The prompt's row is the frame's 4th, after the padding, the label and the 6 typed.
	x := 2 + labelWidth + 6
	if got, want := f.View().Cursor, cursor(x, 4, tea.CursorBar); !reflect.DeepEqual(got, want) {
		t.Errorf("insert: %+v, want %+v", got, want)
	}
	if got, want := p.View().Cursor, cursor(x, 3, tea.CursorBar); !reflect.DeepEqual(got, want) {
		t.Errorf("in a panel: %+v, want %+v", got, want)
	}
	// Normal mode puts the cursor on the last character, as vim does.
	p.Update(keyMsg("esc"))
	if got, want := f.View().Cursor, cursor(x-1, 4, tea.CursorBlock); !reflect.DeepEqual(got, want) {
		t.Errorf("normal: %+v, want %+v", got, want)
	}
	p.Update(tea.KeyPressMsg{Code: '/', Mod: tea.ModCtrl})
	if got := p.View().Cursor; got != nil {
		t.Errorf("under the list of keys: %+v", got)
	}

	f = newLayoutForm(testLayouts(t), clickSpace, clickWt, clickOpen, "agent", nil)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	f.Update(keyMsg("tab"))
	if got := f.View().Cursor; got != nil {
		t.Errorf("without vim: %+v", got)
	}
}

// modeRow is the row of the form's screen that shows a field's vim mode, and the mode, or -1 when none
// does, as while a choice has focus.
func modeRow(f interface{ render() string }) (int, string) {
	for i, l := range formScreen(f) {
		for _, mode := range []string{"NORMAL", "INSERT", "VISUAL"} {
			if strings.HasSuffix(l, " "+mode) {
				return i, mode
			}
		}
	}
	return -1, ""
}

// In normal mode j and k go down and up through the form's fields and choices, as down and up do, each
// field taking normal mode from the one before. A j that vim waits for, as rj's, is vim's.
func TestFieldVimNavigate(t *testing.T) {
	f := newForm(testLayouts(t), clickSpace)
	f.theme().SetVim(true)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	typeInto(f, "fix")
	f.Update(keyMsg("esc"))
	type at struct {
		row  int
		mode string
	}
	steps := []struct {
		keys string
		want at
	}{
		// The branch, the directory, the choice of layout, which shows no mode, then its prompt.
		{"", at{5, "NORMAL"}},
		{"j", at{6, "NORMAL"}},
		{"j", at{-1, ""}},
		{"j", at{8, "NORMAL"}},
		{"kkk", at{5, "NORMAL"}},
		// r takes the j as the character it puts, on the branch's x.
		{"rj", at{5, "NORMAL"}},
	}
	for _, s := range steps {
		typeInto(f, s.keys)
		if row, mode := modeRow(f); !reflect.DeepEqual(at{row, mode}, s.want) {
			t.Errorf("after %q: %v, want %v", s.keys, at{row, mode}, s.want)
		}
	}
	if got, want := f.branch.Value(), "fij"; got != want {
		t.Errorf("branch %q, want %q", got, want)
	}
	// A appends on the branch, and j types there.
	typeInto(f, "Aj")
	if got, want := f.branch.Value(), "fijj"; got != want {
		t.Errorf("branch %q, want %q", got, want)
	}
}

// In a field of several lines j and k move between them, and go on to the next field from its last or
// the one before from its first.
func TestTextFieldVimNavigate(t *testing.T) {
	f := newLayoutForm(testLayouts(t), clickSpace, clickWt, clickOpen, "agent", map[string]string{"prompt": "a\nb"})
	f.theme().SetVim(true)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	f.Update(keyMsg("tab"))
	f.Update(keyMsg("esc"))
	// The prompt's lines are the screen's 4th and 5th, the mode on the first.
	typeInto(f, "k")
	if row, mode := modeRow(f); row != 4 || mode != "NORMAL" || f.focused() == nil {
		t.Errorf("k on the last line: row %d %q, focused %v", row, mode, f.focused())
	}
	typeInto(f, "k")
	if f.focused() != nil {
		t.Error("k on the first line stayed in the prompt")
	}
	typeInto(f, "j")
	if f.focused() == nil {
		t.Error("j on the choice did not go to the prompt")
	}
}
