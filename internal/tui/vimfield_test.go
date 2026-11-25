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
