package tui

import (
	"reflect"
	"testing"

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
		"\u25be 21 more",
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
		"\u25be 19 more",
		"",
	})
	e.press("G")
	check("at the bottom", []string{
		title, "\u25b4 21 more",
		" f       show only what is not",
		"         idle",
		" O       show only open",
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
