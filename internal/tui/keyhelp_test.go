package tui

import (
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// ? lists the keys where the cursor is over the sidebar, the row's first, then the list's and the
// sidebar's; j, k, g and G scroll it, and esc closes it. In the menu it lists the menu's keys, then its
// items', and closing it goes back to the menu.
func TestSidebarHelp(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.m.Update(tea.WindowSizeMsg{Width: 30, Height: 12})
	e.m.cursor = e.row("gr@dev")
	check := func(what string, want []string) {
		t.Helper()
		if got := e.screen(); !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n%q\nwant\n%q", what, got, want)
		}
	}
	title := "KEYS       j/k scroll  ? close"
	e.press("?")
	check("the row's keys", []string{
		title, menuRule("gr"),
		" enter/l open",
		" space   fold or unfold the",
		"         space's worktrees",
		" n       new worktree",
		" N       new worktree from PR",
		" I       new worktree from",
		"         issue",
		" c       close session",
		"\u25be 23 more",
		"",
	})
	e.press("j", "j")
	check("scrolled two lines", []string{
		title, "\u25b4 3 more",
		"         space's worktrees",
		" n       new worktree",
		" N       new worktree from PR",
		" I       new worktree from",
		"         issue",
		" c       close session",
		" x       remove worktree",
		menuRule("Spaces"),
		"\u25be 21 more",
		"",
	})
	e.press("G")
	check("at the bottom", []string{
		title, "\u25b4 23 more",
		" O       show only open",
		"         sessions",
		" Z       hide suspended",
		"         sessions",
		" P       open the picker",
		" s       change the sort mode",
		" K/J     move the row up/down",
		" v       select rows to act on",
		" ?       list keys",
		"",
	})
	e.press("esc")
	if e.m.help != nil {
		t.Fatal("esc left the keys open")
	}

	e.press("m", "?")
	check("the menu's keys", []string{
		title, menuRule("Menu"),
		" j/k     down/up",
		" enter/l run",
		" esc/q/m close",
		" ?       list keys",
		" ctrl+c  quit",
		menuRule("gr"),
		" space   fold or unfold the",
		"         space's worktrees",
		"\u25be 8 more",
		"",
	})
	e.press("?")
	if e.m.help != nil || e.m.menu == nil {
		t.Errorf("? again: keys open %v, menu open %v; want the menu back", e.m.help != nil, e.m.menu != nil)
	}
}

// In a popup ? is typed into a field, and lists the form's keys in a sheet up from the bottom while a
// choice has focus. esc closes the sheet, not the form.
func TestPopupHelp(t *testing.T) {
	p := newWorktreePanel(t)
	question := tea.KeyPressMsg{Code: '?', Text: "?"}
	p.Update(question)
	if f := p.inner.(*newWorktreeForm); p.help != nil || f.branch.Value() != "pr/me/?" {
		t.Fatalf("? in the branch field: keys open %v, branch %q", p.help != nil, f.branch.Value())
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	// Round the fields to the tabs.
	p.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	p.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	p.Update(question)
	row := func(s string) string { return "│ " + s + strings.Repeat(" ", 86-utf8.RuneCountInString(s)) + " │" }
	want := []string{
		"╭─ New worktree in gr " + strings.Repeat("─", 67) + "╮",
		row(""),
		row(" Branch   Pull request   Issue   left/right switch"),
		row(""),
		row("branch     pr/me/"),
		row("directory  .worktrees"),
		row(""),
		row("Name the branch to create or check out, or # for a PR"),
	}
	for len(want) < 15 {
		want = append(want, row(""))
	}
	want = append(want,
		row(strings.Repeat("─", 86)),
		row("KEYS"+strings.Repeat(" ", 75)+"? close"),
		row("─ New worktree "+strings.Repeat("─", 71)),
		row(" tab        next field"),
		row(" shift+tab  previous field"),
		row(" left/right choose"),
		row(" enter      create"),
		row(" esc        cancel"),
		"╰"+strings.Repeat("─", 88)+"╯",
	)
	if got := panelScreen(p); !reflect.DeepEqual(got, want) {
		t.Errorf("the form's keys:\n%q\nwant\n%q", got, want)
	}
	if _, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape}); p.help != nil || quits(cmd) {
		t.Errorf("esc: keys open %v, quit %v; want the keys closed and the form open", p.help != nil, quits(cmd))
	}
}

// In the picker ? lists its keys while the query is empty, and is typed into one that is not, where ctrl+/
// lists them; with the menu open it lists the menu's keys and its items'.
func TestPickerHelp(t *testing.T) {
	m := newPicker(0)
	p, _ := stubPopup(m, 0, true)
	p.Update(tea.WindowSizeMsg{Width: 60, Height: 26})
	p.Update(fixture(time.Now()))
	question := tea.KeyPressMsg{Code: '?', Text: "?"}
	// sheet is the screen from the rule above the keys' title.
	sheet := func() []string {
		screen := panelScreen(p)
		for i, l := range screen {
			if strings.HasPrefix(l, "KEYS") {
				return screen[i-1:]
			}
		}
		return nil
	}
	p.Update(question)
	want := []string{
		strings.Repeat("─", 60),
		"KEYS" + strings.Repeat(" ", 49) + "? close",
		"─ Picker " + strings.Repeat("─", 51),
		" down/up down/up",
		" enter   switch",
		" tab     actions",
		" esc     cancel",
	}
	if got := sheet(); !reflect.DeepEqual(got, want) {
		t.Errorf("the picker's keys:\n%q\nwant\n%q", got, want)
	}
	p.Update(question)
	typeInto(p, "gr")
	p.Update(question)
	if p.help != nil || m.input.Value() != "gr?" {
		t.Errorf("? after a query: keys open %v, query %q", p.help != nil, m.input.Value())
	}
	// ctrl+/ lists them all the same, and ctrl+_, as a terminal without kitty's keyboard protocol sends it.
	p.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	for _, k := range []tea.KeyPressMsg{{Code: '/', Mod: tea.ModCtrl}, {Code: '_', Mod: tea.ModCtrl}} {
		p.Update(k)
		if p.help == nil || m.input.Value() != "gr" {
			t.Errorf("%s after a query: keys open %v, query %q", k, p.help != nil, m.input.Value())
		}
		p.Update(k)
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	p.Update(question)
	want = []string{
		strings.Repeat("─", 60),
		"KEYS" + strings.Repeat(" ", 49) + "? close",
		"─ Menu " + strings.Repeat("─", 53),
		" down/up down/up",
		" enter   run",
		" esc/tab close",
		" ctrl+c  quit",
		"─ gr " + strings.Repeat("─", 55),
		" c       close session",
		" n       new worktree",
		" N       new worktree from PR",
		" I       new worktree from issue",
		" p       clean up worktrees",
	}
	if got := sheet(); !reflect.DeepEqual(got, want) {
		t.Errorf("the menu's keys:\n%q\nwant\n%q", got, want)
	}
}

// While the sidebar's filter is open ? is typed into it, and ctrl+/ lists the filter's keys.
func TestFilterHelp(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.press("/", "?")
	if e.m.help != nil || e.m.filter.Value() != "?" {
		t.Fatalf("? in the filter: keys open %v, filter %q", e.m.help != nil, e.m.filter.Value())
	}
	e.m.Update(tea.KeyPressMsg{Code: '/', Mod: tea.ModCtrl})
	if got := e.screen()[:8]; !reflect.DeepEqual(got, []string{
		"KEYS" + strings.Repeat(" ", 19) + "? close",
		menuRule("Filter"),
		" down/up down/up",
		" enter   open",
		" esc     clear",
		" ctrl+/  list keys",
		" ctrl+c  quit",
		"",
	}) {
		t.Errorf("the filter's keys:\n%q", got)
	}
	e.press("esc")
	if e.m.help != nil || !e.m.filtering || e.m.filter.Value() != "?" {
		t.Errorf("esc: keys open %v, filtering %v, filter %q; want the keys closed and the filter as it was", e.m.help != nil, e.m.filtering, e.m.filter.Value())
	}
}
