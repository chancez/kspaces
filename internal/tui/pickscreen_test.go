package tui

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
)

// screenFixture is previewFixture with each session's screen, the picker's own window, 30, reporting a
// status, and a form's.
func screenFixture() spaces.Snapshot {
	snap := previewFixture()
	snap.Kitty.Screens = map[string]int{"gr": 10, "gr@dev": 11, "default": 12, "docs@notes": 13}
	snap.Kitty.Statuses[1].Window = 30
	snap.Kitty.Own = map[int]bool{23: true}
	return snap
}

// screenWindows is the window whose screen the preview shows for each entry, from the first.
func screenWindows(m *picker) []int {
	m.cursor = 0
	var got []int
	for range m.matches {
		got = append(got, m.screenWindow())
		down(m, 1)
	}
	return got
}

// A status entry shows its own window's screen, a session the one kitty would focus, a session with no
// windows none, and neither the picker's own window nor another of kspaces'. Below the list only an entry
// with few details leaves room for a screen.
func TestPickerScreenWindow(t *testing.T) {
	m := newPicker(30)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(screenFixture())
	// gr@dev, scratch, docs@notes and default's statuses, then default, gr, gr / dev, gr / fix, kitty, docs
	// and docs / notes.
	if got, want := screenWindows(m), []int{20, 0, 22, 0, 12, 10, 11, 0, 0, 0, 13}; !reflect.DeepEqual(got, want) {
		t.Errorf("beside: got %v, want %v", got, want)
	}
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 26})
	if got, want := screenWindows(m), []int{20, 0, 22, 0, 0, 0, 0, 0, 0, 0, 0}; !reflect.DeepEqual(got, want) {
		t.Errorf("below: got %v, want %v", got, want)
	}
}

// The screen's block is its last rows of text, blank rows after them dropped, each cut to width with its
// colors, which the rows before set and each row ends. Nothing but colors and styles is left: no prompt
// mark, hyperlink or cursor move.
func TestScreenBlock(t *testing.T) {
	text := "\x1b]133;A\x1b\\$ make\n" +
		"\x1b[32mok\x1b[39m  first\n" +
		"\x1b[31;1merror:\x1b[22m a line longer than the preview is wide\n" +
		"\x1b]8;;file:///x\x1b\\main.go\x1b]8;;\x1b\\:3\x1b[2K\x1b[1A\n" +
		"\x1b[m\x1b]133;A\x1b\\$ \n\n   \n"
	got := screenBlock(screenLines(text), 12, 3)
	want := []string{
		"\x1b[32m\x1b[39m\x1b[31;1merror:\x1b[22m a lin\x1b[m",
		"\x1b[32m\x1b[39m\x1b[31;1m\x1b[22mmain.go:3\x1b[m",
		"\x1b[32m\x1b[39m\x1b[31;1m\x1b[22m\x1b[m$ \x1b[m",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("block:\n%q\nwant\n%q", got, want)
	}
	if got := screenBlock(screenLines("one\ntwo\n"), 12, 3); !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Errorf("short screen: %q", got)
	}
}

// fakeScreens fetches screens from texts, a window with none failing, and records which were fetched.
type fakeScreens struct {
	texts map[int]string
	asked []int
}

func (f *fakeScreens) get(window int) (string, error) {
	f.asked = append(f.asked, window)
	text, ok := f.texts[window]
	if !ok {
		return "", errors.New("no window")
	}
	return text, nil
}

// The preview follows the cursor's window, a fetch at a time: a screen for an entry the cursor has left
// is dropped and the entry it is on fetched; the refresh fetches again only while the cursor stays, and
// shows what changed. A failed fetch shows no screen.
func TestPickerScreenFollows(t *testing.T) {
	screens := &fakeScreens{texts: map[int]string{
		20: "$ claude\n\x1b[33mwants permission to run\x1b[m a command that is longer than the preview\n\n",
		22: "$ make\n",
	}}
	m := newPicker(30)
	m.getText = screens.get
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 16})
	_, first := m.Update(screenFixture())

	// The cursor goes to docs@notes's status and back while gr@dev's first fetch runs.
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown}); cmd != nil {
		t.Errorf("a second fetch while one runs: %v", cmd())
	}
	down(m, 1)
	_, toNotes := m.Update(first())
	if m.screen.lines != nil {
		t.Errorf("a screen for an entry the cursor left: %q", m.screen.lines)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	_, toDev := m.Update(toNotes())
	msg := toDev()
	if want := (screenMsg{window: 20, seq: 5, text: screens.texts[20]}); msg != want {
		t.Errorf("fetched %#v, want %#v", msg, want)
	}
	if _, tick := m.Update(msg); tick == nil {
		t.Error("no refresh after a fetch")
	}

	beside := func(list, preview string) string {
		return strings.TrimRight(fmt.Sprintf("%-48s \u2502 %s", list, preview), " ")
	}
	list := []string{
		blocked + "gr@dev  approve",
		"\ue654  scratch  exit 1",
		"\uf110  docs@notes  make",
		"\uf10c  default  claude",
		open + "default",
		open + "gr  main",
		blocked + "gr / dev  pr/me/dev",
		"   gr / fix  fix",
		"   kitty  master",
		"   docs  main",
		"\uf110  docs / notes  notes",
		"",
		"",
	}
	screen := func(shown ...string) []string {
		preview := append([]string{
			fmt.Sprintf("%-44s+1 5m", blocked+"dev"),
			"   claude",
			"   wants permission",
			"   approve",
			"   id deploy",
			"   working build",
		}, shown...)
		out := []string{"\uf002  space, worktree or status", strings.Repeat("\u2500", 100)}
		for i, l := range list {
			p := ""
			if i < len(preview) {
				p = preview[i]
			}
			out = append(out, beside(l, p))
		}
		return append(out, "11/11")
	}
	want := screen(strings.Repeat("\u2500", 49), "$ claude", "wants permission to run a command that is longer")
	if got := pickerScreen(m); !reflect.DeepEqual(got, want) {
		t.Errorf("screen:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := strings.Split(m.render(), "\n")[10]; !strings.Contains(got, "\x1b[33mwants permission to run\x1b[m a command") {
		t.Errorf("colors lost: %q", got)
	}

	// A tick from before the cursor moved fetches nothing; the one for this entry fetches what changed.
	if _, cmd := m.Update(screenTickMsg{window: 20, seq: 1}); cmd != nil {
		t.Errorf("a stale tick fetched: %v", cmd())
	}
	screens.texts[20] = "$ claude\ndone\n"
	_, cmd := m.Update(screenTickMsg{window: 20, seq: 5})
	m.Update(cmd())
	want = screen(strings.Repeat("\u2500", 49), "$ claude", "done")
	if got := pickerScreen(m); !reflect.DeepEqual(got, want) {
		t.Errorf("refreshed:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// The window closed: the next fetch fails and the preview shows no screen.
	delete(screens.texts, 20)
	_, cmd = m.Update(screenTickMsg{window: 20, seq: 5})
	m.Update(cmd())
	if got, want := pickerScreen(m), screen(); !reflect.DeepEqual(got, want) {
		t.Errorf("failed:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// Leaving the entry ends its refresh.
	down(m, 1)
	if _, cmd := m.Update(screenTickMsg{window: 20, seq: 5}); cmd != nil {
		t.Errorf("a tick after the cursor left fetched: %v", cmd())
	}
	if want := []int{20, 22, 20, 20, 20}; !reflect.DeepEqual(screens.asked, want) {
		t.Errorf("fetched %v, want %v", screens.asked, want)
	}
}
