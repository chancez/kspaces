package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/status"
	"github.com/chancez/kspaces/internal/tui/interact"
	"github.com/chancez/kspaces/internal/tui/look"
)

// fixture is what the sidebar shows in tests: a session in no space; gr with two linked worktrees, the
// sidebar in gr@dev; kitty with none; docs folded; a space that is not a repository; and a status entry
// in each of four states, gr@dev's blocked with another record.
func fixture(now time.Time) spaces.Snapshot {
	progress := 40
	cfg := config.Config{}
	return spaces.Snapshot{
		Config: &cfg,
		Spaces: []spaces.Space{
			{Name: "gr", Root: "/src/gr", Worktrees: []spaces.Worktree{
				{Path: "/src/gr", Branch: "main", Session: "gr"},
				{Path: "/src/gr/.worktrees/dev", Branch: "pr/me/dev", Session: "gr@dev"},
				{Path: "/src/gr/.worktrees/fix", Branch: "fix", Session: "gr@fix"},
			}},
			{Name: "kitty", Root: "/src/kitty", Worktrees: []spaces.Worktree{{Path: "/src/kitty", Branch: "master", Session: "kitty"}}},
			{Name: "docs", Root: "/src/docs", Worktrees: []spaces.Worktree{
				{Path: "/src/docs", Branch: "main", Session: "docs"},
				{Path: "/src/docs/.worktrees/notes", Branch: "notes", Session: "docs@notes"},
			}},
			{Name: "broken", Root: "/src/broken", Err: errors.New("not a git repository")},
		},
		Others: []spaces.Space{{Name: "default", Worktrees: []spaces.Worktree{{Session: "default"}}}},
		Folded: map[string]bool{"/src/docs": true},
		Kitty: kitty.State{
			Sessions: map[string]int{"gr": 10, "gr@dev": 11, "default": 12, "docs@notes": 13},
			Windows:  map[string]int{"gr": 1, "gr@dev": 2, "default": 1, "docs@notes": 1},
			Current:  "gr@dev",
			Home:     11,
			Statuses: []status.Status{
				{
					Window: 20, Session: "gr@dev",
					Record: status.Record{ID: "deploy", State: "blocked", Kind: "permission", App: "claude", Title: "approve", Since: now.Add(-5 * time.Minute)},
					Others: []status.Record{{ID: "build", State: "working"}},
				},
				{Window: 21, WindowTitle: "scratch", Record: status.Record{State: "error", Msg: "exit 1"}},
				{Window: 22, Session: "docs@notes", Record: status.Record{State: "working", App: "make", Progress: &progress, Since: now.Add(-3 * time.Hour)}},
				{Window: 23, Session: "default", Record: status.Record{State: "idle", App: "claude"}},
			},
		},
	}
}

type sidebarEnv struct {
	t *testing.T
	m *sidebar
}

// newSidebarEnv is a sidebar 30 cells wide and 40 high showing fixture, with the state files and the
// config in directories of the test's own, so moving a space never edits mine.
func newSidebarEnv(t *testing.T) *sidebarEnv {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	e := &sidebarEnv{t: t, m: newSidebar(0)}
	e.m.Update(tea.WindowSizeMsg{Width: 30, Height: 40})
	e.m.Update(fixture(time.Now()))
	e.m.render()
	return e
}

func keyMsg(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	}
	if rest, ok := strings.CutPrefix(s, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: rune(rest[0]), Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

// press sends keys, ignoring the commands they return, and draws the screen after each, as the program
// would.
func (e *sidebarEnv) press(keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		_, cmd = e.m.Update(keyMsg(k))
		e.m.render()
	}
	return cmd
}

// screen is the frame without its styles, a line each, without the border.
func (e *sidebarEnv) screen() []string {
	var out []string
	for _, l := range strings.Split(ansi.Strip(e.m.render()), "\n") {
		out = append(out, strings.TrimRight(strings.TrimSuffix(l, "\u2502"), " "))
	}
	return out
}

// highlighted are the lines drawn in reverse video, the cursor's bar, without their styles.
func (e *sidebarEnv) highlighted() []string {
	var out []string
	for _, l := range strings.Split(e.m.render(), "\n") {
		if strings.Contains(l, "\x1b[7m") {
			out = append(out, strings.TrimRight(strings.TrimSuffix(ansi.Strip(l), "\u2502"), " "))
		}
	}
	return out
}

// zone is where the last frame drew zone id.
func (e *sidebarEnv) zone(id string) (x, y int) {
	e.t.Helper()
	zones := e.m.zones[id]
	if len(zones) == 0 {
		e.t.Fatalf("no zone %q on screen:\n%s", id, strings.Join(e.screen(), "\n"))
	}
	return zones[0].X, zones[0].Y
}

// row is the index of the spaces row of session, folded or not.
func (e *sidebarEnv) row(session string) int {
	e.t.Helper()
	for i, r := range e.m.rows {
		if r.Worktree != nil && r.Worktree.Session == session {
			return i
		}
	}
	e.t.Fatalf("no row for %s", session)
	return 0
}

// entry is the cursor index of the status entry of window id.
func (e *sidebarEnv) entry(window int) int {
	e.t.Helper()
	for j, s := range e.m.kitty.Statuses {
		if s.Window == window {
			return len(e.m.rows) + j
		}
	}
	e.t.Fatalf("no status entry for window %d", window)
	return 0
}

// click presses button on zone id and lets go of it, then draws the screen.
func (e *sidebarEnv) click(id string, button tea.MouseButton) tea.Cmd {
	e.t.Helper()
	x, y := e.zone(id)
	return e.clickAt(x, y, button)
}

// clickAt is the command of the press, or of the release when that runs one, as opening a row does.
func (e *sidebarEnv) clickAt(x, y int, button tea.MouseButton) tea.Cmd {
	_, cmd := e.m.Update(tea.MouseClickMsg{X: x, Y: y, Button: button})
	if _, done := e.m.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: button}); done != nil {
		cmd = done
	}
	e.m.render()
	return cmd
}

// hover moves the mouse onto zone id, then draws the screen.
func (e *sidebarEnv) hover(id string) {
	e.t.Helper()
	x, y := e.zone(id)
	e.m.Update(tea.MouseMotionMsg{X: x, Y: y})
	e.m.render()
}

// tick is the refresh timer firing, then the screen drawn.
func (e *sidebarEnv) tick() {
	e.m.Update(tickMsg{})
	e.m.render()
}

func (e *sidebarEnv) has(line string) bool {
	for _, l := range e.screen() {
		if l == line {
			return true
		}
	}
	return false
}

func (e *sidebarEnv) folded() string {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join(os.Getenv("XDG_STATE_HOME"), "kspaces", "folded"))
	if err != nil && !os.IsNotExist(err) {
		e.t.Fatal(err)
	}
	return string(data)
}

// Row icons, each with the two spaces a marker takes.
const (
	open       = "\uf444  "
	blocked    = "\uf071  "
	repoIcon   = "\uf401  "
	branchIcon = "\ue725  "
	termIcon   = "\uf120  "
)

// spacesTitle is the SPACES title with the sort button, manual, and the add button in the last of the 29
// columns before the border.
const spacesTitle = "SPACES" + "                    " + "\uf0c9 \uf067"

// The spaces list: sessions in no space, then each space and its linked worktrees. A row starts with
// the fold caret, or as wide a blank, then its icon: what the row is, or the most urgent state of its
// sessions, or a cross for a broken space.
func TestSpacesList(t *testing.T) {
	e := newSidebarEnv(t)
	want := []string{
		spacesTitle,
		"",
		"  " + termIcon + "default",
		"",
		"\u25bc " + repoIcon + "gr main",
		"  " + blocked + "dev pr/me/dev",
		"  " + branchIcon + "fix fix",
		"",
		"  " + repoIcon + "kitty master",
		"",
		"\u25b6 \uf110  docs main",
		"",
		"  \uf00d  broken",
	}
	if got := e.screen()[:len(want)]; !reflect.DeepEqual(got, want) {
		t.Errorf("spaces list:\n%q\nwant\n%q", got, want)
	}
	// The sidebar's own session is a bar, the cursor shown only while navigating.
	if got := e.highlighted(); got != nil {
		t.Errorf("highlighted without focus: %q", got)
	}
}

// The status list is grouped by state, most urgent first, each group titled with its count; an entry
// shows its window's label, its other records and its age, and one row of what the program said.
func TestStatusGroupsOnScreen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newSidebarEnv(t)
		var got []string
		for i, l := range e.screen() {
			if l == "STATUS" {
				got = e.screen()[i : i+18]
				break
			}
		}
		want := []string{
			"STATUS",
			"Blocked 1",
			blocked + "dev                  +1 5m",
			"   claude \u00b7 approve",
			"",
			"Failed 1",
			"\ue654  scratch",
			"   exit 1",
			"",
			"Working 1",
			"\uf110  notes                   3h",
			"   make \u00b7 \u2588\u2588\u2588\u2588\u2588\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591 40%",
			"",
			"Idle 1",
			"\uf10c  default",
			"   claude",
			"",
			"",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("status list:\n%q\nwant\n%q", got, want)
		}

		// With nothing to report, the list says so.
		snap := fixture(time.Now())
		snap.Kitty.Statuses = nil
		e.m.Update(snap)
		if !e.has("Nothing to report") {
			t.Errorf("no note for an empty status list:\n%s", strings.Join(e.screen(), "\n"))
		}
	})
}

// Clicking a space's caret folds its linked worktrees into its row, for every sidebar, and clicking it
// again shows them. Clicking the name opens it instead.
func TestFoldCaretClick(t *testing.T) {
	e := newSidebarEnv(t)
	gr := e.row("gr")
	if cmd := e.click(fmt.Sprintf(caretZone, gr), tea.MouseLeft); cmd != nil {
		t.Error("the caret opened the space as well as folding it")
	}
	// gr@dev is the current session, so the folded row stands for it: its state and its bar.
	if !e.has("\u25b6 "+blocked+"gr main") || e.has("  "+blocked+"dev pr/me/dev") {
		t.Errorf("gr is not folded:\n%s", strings.Join(e.screen(), "\n"))
	}
	if got, want := e.folded(), "/src/docs\n/src/gr\n"; got != want && got != "/src/gr\n/src/docs\n" {
		t.Errorf("folded file = %q, want %q", got, want)
	}
	e.click(fmt.Sprintf(caretZone, e.row("gr")), tea.MouseLeft)
	if !e.has("\u25bc "+repoIcon+"gr main") || !e.has("  "+branchIcon+"fix fix") {
		t.Errorf("gr is not unfolded:\n%s", strings.Join(e.screen(), "\n"))
	}
	if got := e.folded(); got != "/src/docs\n" {
		t.Errorf("folded file = %q", got)
	}

	// The name, past the caret, opens the space's main worktree.
	x, y := e.zone(fmt.Sprintf(entryZone, e.row("gr")))
	if cmd := e.clickAt(x+6, y, tea.MouseLeft); cmd == nil || e.m.cursor != e.row("gr") {
		t.Errorf("clicking gr's name: cursor %d, opened %v", e.m.cursor, cmd != nil)
	}
	if !e.has("\u25bc " + repoIcon + "gr main") {
		t.Error("clicking the name folded the space")
	}
}

// space folds the space of the cursor's row, from a linked worktree too, and leaves a space without
// linked worktrees alone.
func TestFoldKey(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.press("space")
	if got := e.highlighted(); !reflect.DeepEqual(got, []string{"\u25b6 " + blocked + "gr main"}) {
		t.Errorf("space on gr@dev: cursor on %q", got)
	}
	e.press("space", "j", "j", "j", "space")
	if got := e.highlighted(); !reflect.DeepEqual(got, []string{"  " + repoIcon + "kitty master"}) || e.folded() != "/src/docs\n" {
		t.Errorf("space on kitty: cursor on %q, folded %q", got, e.folded())
	}
}

// menuRule is a menu's rule in the 30 cell sidebar, with a section's name on it, or plain for "".
func menuRule(head string) string {
	if head == "" {
		return strings.Repeat("\u2500", 29)
	}
	return "\u2500 " + head + " " + strings.Repeat("\u2500", 29-3-len(head))
}

// A right click, or m, opens the row's menu below it, its items under their sections; the mouse
// highlights an item, and a click runs it.
func TestRightClickMenu(t *testing.T) {
	e := newSidebarEnv(t)
	dev := e.row("gr@dev")
	e.click(fmt.Sprintf(entryZone, dev), tea.MouseRight)
	_, y := e.zone(fmt.Sprintf(entryZone, dev))
	want := []string{
		menuRule("Session"), " Open", " Save as layout", " Close session",
		menuRule("Worktree"), " Remove worktree",
		menuRule("Space"), " New worktree", " Fold worktrees", " Add space",
		menuRule(""),
	}
	if got := e.screen()[y+1 : y+12]; !reflect.DeepEqual(got, want) {
		t.Errorf("menu:\n%q\nwant\n%q", got, want)
	}
	if got := e.highlighted(); !reflect.DeepEqual(got, []string{" Open"}) {
		t.Errorf("highlighted %q, want the first item", got)
	}
	e.hover(fmt.Sprintf(menuZone, 5))
	if got := e.highlighted(); !reflect.DeepEqual(got, []string{" Fold worktrees"}) {
		t.Errorf("highlighted %q after moving onto Fold worktrees", got)
	}
	if cmd := e.click(fmt.Sprintf(menuZone, 5), tea.MouseLeft); cmd != nil || e.m.menu != nil {
		t.Errorf("running Fold worktrees: cmd %v, menu still open %v", cmd != nil, e.m.menu != nil)
	}
	if !e.has("\u25b6 "+blocked+"gr main") || e.m.cursor != e.row("gr") {
		t.Errorf("Fold worktrees did not fold gr onto the cursor:\n%s", strings.Join(e.screen(), "\n"))
	}

	// What a row offers depends on it: a session in no space can only be opened and closed, besides
	// what every row offers.
	def := e.row("default")
	e.click(fmt.Sprintf(entryZone, def), tea.MouseRight)
	_, y = e.zone(fmt.Sprintf(entryZone, def))
	want = []string{menuRule("Session"), " Open", " Close session", menuRule("Space"), " Add space", menuRule("")}
	if got := e.screen()[y+1 : y+7]; !reflect.DeepEqual(got, want) {
		t.Errorf("default's menu:\n%q\nwant\n%q", got, want)
	}
	// A click elsewhere closes it; a right click elsewhere opens that row's instead.
	e.clickAt(5, 0, tea.MouseLeft)
	if e.m.menu != nil {
		t.Error("a click elsewhere left the menu open")
	}
	e.click(fmt.Sprintf(entryZone, e.row("kitty")), tea.MouseRight)
	if e.m.menu == nil || e.m.cursor != e.row("kitty") {
		t.Error("a right click on kitty did not open its menu")
	}
}

// A menu of one section draws no name on its rules: a space that is not a repository has only what every
// row offers.
func TestMenuOneSection(t *testing.T) {
	e := newSidebarEnv(t)
	broken := len(e.m.rows) - 1
	e.click(fmt.Sprintf(entryZone, broken), tea.MouseRight)
	_, y := e.zone(fmt.Sprintf(entryZone, broken))
	if got, want := e.screen()[y+1:y+4], []string{menuRule(""), " Add space", menuRule("")}; !reflect.DeepEqual(got, want) {
		t.Errorf("the broken space's menu:\n%q\nwant\n%q", got, want)
	}
}

// In a narrow sidebar the menu keeps to its width, the sections' names cut before the items.
func TestMenuNarrow(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.WindowSizeMsg{Width: 12, Height: 40})
	e.m.render()
	dev := e.row("gr@dev")
	e.click(fmt.Sprintf(entryZone, dev), tea.MouseRight)
	_, y := e.zone(fmt.Sprintf(entryZone, dev))
	rule := strings.Repeat("\u2500", 11)
	want := []string{
		"\u2500 Session \u2500", " Open", " Save as...", " Close s...",
		"\u2500 Work... \u2500", " Remove ...",
		"\u2500 Space \u2500\u2500\u2500", " New wor...", " Fold wo...", " Add space",
		rule,
	}
	if got := e.screen()[y+1 : y+12]; !reflect.DeepEqual(got, want) {
		t.Errorf("menu:\n%q\nwant\n%q", got, want)
	}
}

// m opens the cursor's menu, j and k move in it, enter runs an item and esc closes it. Keys it does not
// take do nothing while it is open.
func TestMenuKeys(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.press("m", "j", "j", "j", "j", "j", "j", "k")
	if got := e.highlighted(); !reflect.DeepEqual(got, []string{"  " + blocked + "dev pr/me/dev", " Fold worktrees"}) {
		t.Errorf("highlighted %q", got)
	}
	e.press("G", "esc")
	if e.m.menu != nil || e.m.cursor != e.row("gr@dev") {
		t.Errorf("menu open %v, cursor %d", e.m.menu != nil, e.m.cursor)
	}
	e.press("m", "j", "j", "j", "j", "j", "enter")
	if !e.has("\u25b6 " + blocked + "gr main") {
		t.Errorf("enter on Fold worktrees did not fold gr:\n%s", strings.Join(e.screen(), "\n"))
	}
}

// The sections' names are not items: j and k step over them, and the mouse on one, or a click, leaves
// the menu as it was.
func TestMenuSkipsSections(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.press("m", "j", "j", "j")
	if got := e.highlighted(); !reflect.DeepEqual(got, []string{"  " + blocked + "dev pr/me/dev", " Remove worktree"}) {
		t.Errorf("j past Session's last item: highlighted %q", got)
	}
	e.press("j", "k")
	if got := e.highlighted(); !reflect.DeepEqual(got, []string{"  " + blocked + "dev pr/me/dev", " Remove worktree"}) {
		t.Errorf("k from Space's first item: highlighted %q", got)
	}
	_, y := e.zone(fmt.Sprintf(menuZone, 3))
	head := y - 1
	if got := e.screen()[head]; got != menuRule("Worktree") {
		t.Fatalf("above Remove worktree is %q, want Worktree's rule", got)
	}
	e.m.Update(tea.MouseMotionMsg{X: 3, Y: head})
	e.m.render()
	if got := e.highlighted(); !reflect.DeepEqual(got, []string{"  " + blocked + "dev pr/me/dev", " Remove worktree"}) {
		t.Errorf("mouse on Worktree's name: highlighted %q", got)
	}
	if cmd := e.clickAt(3, head, tea.MouseLeft); cmd != nil || e.m.menu == nil || e.m.menu.sel != 3 {
		t.Errorf("click on Worktree's name: cmd %v, menu open %v", cmd != nil, e.m.menu != nil)
	}
}

// The sidebar losing focus, as to a click in another window, closes its menu.
func TestMenuClosesOnBlur(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.m.Update(tea.BlurMsg{})
	want := e.screen()
	e.m.Update(tea.FocusMsg{})
	e.click(fmt.Sprintf(entryZone, e.row("gr@dev")), tea.MouseRight)
	if e.m.menu == nil {
		t.Fatal("the right click opened no menu")
	}
	e.m.Update(tea.BlurMsg{})
	if got := e.screen(); e.m.menu != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("menu open %v after the blur:\n%q\nwant\n%q", e.m.menu != nil, got, want)
	}
}

// a adds a space wherever the cursor is: on a row, on a status entry, or in a sidebar with neither.
func TestAddSpaceKey(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	adds := func() bool {
		b, ok := interact.Lookup(e.m, keyMsg("a"), e.m.chain()...)
		return ok && b.Is(actAddSpace)
	}
	got := []bool{adds()}
	e.press("G")
	got = append(got, adds())
	e.m.Update(spaces.Snapshot{Config: &config.Config{}})
	got = append(got, adds())
	if want := []bool{true, true, true}; !reflect.DeepEqual(got, want) {
		t.Errorf("a adds a space on a row, a status entry, nothing: %v, want %v", got, want)
	}
}

// fakeKitten puts a kitten on PATH that records its arguments, and returns them as the last call had them.
func fakeKitten(t *testing.T) func() []string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "args")
	script := "#!/bin/sh\nfor a; do printf '%s\\n' \"$a\"; done > " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "kitten"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	return func() []string {
		data, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	}
}

// The plus at the right end of SPACES adds a space as a does, lit while the mouse is on it; a click on
// the rest of the title does nothing. The narrowest sidebar keeps both.
func TestAddSpaceButton(t *testing.T) {
	e := newSidebarEnv(t)
	launched := fakeKitten(t)
	if got := e.screen()[0]; got != spacesTitle {
		t.Errorf("title %q, want %q", got, spacesTitle)
	}
	lit := e.m.th.Render(look.Plain, look.AddButton(true))
	e.hover(addZone)
	if got := strings.Split(e.m.render(), "\n")[0]; !strings.Contains(got, lit) {
		t.Errorf("button under the mouse not lit: %q", got)
	}
	e.m.Update(tea.MouseMotionMsg{X: 3, Y: 0})
	if got := strings.Split(e.m.render(), "\n")[0]; strings.Contains(got, lit) {
		t.Errorf("button lit with the mouse off it: %q", got)
	}

	if cmd := e.clickAt(3, 0, tea.MouseLeft); cmd != nil {
		t.Errorf("a click on the title ran %v", cmd())
	}
	if got := launched(); got != nil {
		t.Errorf("a click on the title launched %q", got)
	}
	cmd := e.click(addZone, tea.MouseLeft)
	if cmd == nil {
		t.Fatal("a click on the button ran nothing")
	}
	if msg := cmd(); msg != nil {
		t.Errorf("adding a space said %v", msg)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := launched(), []string{"@", "launch", "--type=overlay", "--var=kspaces_form=1", "--next-to", "id:11", exe, "add"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the button launched %q, want %q", got, want)
	}

	e.m.Update(tea.WindowSizeMsg{Width: 12, Height: 40})
	if got, want := e.screen()[0], "SPACES  \uf0c9 \uf067"; got != want {
		t.Errorf("narrowest title %q, want %q", got, want)
	}
}

// A menu that would run off the bottom opens above its row.
func TestMenuAbove(t *testing.T) {
	e := newSidebarEnv(t)
	// Only gr, so its last row is the list's last, and no indicator takes it, below sessions in no space,
	// so the menu has room above.
	snap := fixture(time.Now())
	snap.Spaces = snap.Spaces[:1]
	for _, name := range []string{"notes", "scratch", "mail"} {
		snap.Others = append(snap.Others, spaces.Space{Name: name, Worktrees: []spaces.Worktree{{Session: name}}})
	}
	e.m.Update(snap)
	e.m.Update(tea.WindowSizeMsg{Width: 30, Height: 18})
	e.m.render()
	fix := e.row("gr@fix")
	_, y := e.zone(fmt.Sprintf(entryZone, fix))
	e.click(fmt.Sprintf(entryZone, fix), tea.MouseRight)
	want := []string{
		menuRule("Session"), " Open",
		menuRule("Worktree"), " Remove worktree",
		menuRule("Space"), " New worktree", " Fold worktrees", " Add space",
		menuRule(""),
	}
	if got := e.screen()[y-9 : y]; !reflect.DeepEqual(got, want) {
		t.Errorf("menu above row %d:\n%q\nwant\n%q", y, got, want)
	}
}

// Keys that arrive without focus, from kitty's send-key, show the cursor from the current session until
// enter, esc, or ten seconds without a key.
func TestRemoteNavigation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newSidebarEnv(t)
		e.press("j")
		if got := e.highlighted(); !reflect.DeepEqual(got, []string{"  " + branchIcon + "fix fix"}) {
			t.Fatalf("after j: cursor on %q", got)
		}
		time.Sleep(9 * time.Second)
		e.press("k")
		time.Sleep(9 * time.Second)
		e.tick()
		if got := e.highlighted(); len(got) != 1 {
			t.Errorf("cursor gone 9s after the last key: %q", got)
		}
		time.Sleep(2 * time.Second)
		e.tick()
		if got := e.highlighted(); got != nil {
			t.Errorf("cursor still drawn 11s after the last key: %q", got)
		}

		// esc ends it at once, without leaving: focus never left the window the keys came from.
		e.press("j")
		if cmd := e.press("esc"); cmd != nil || e.highlighted() != nil {
			t.Errorf("esc: leaves %v, cursor %q", cmd != nil, e.highlighted())
		}
		// enter opens the cursor's entry and ends it, even where there is nothing to open.
		e.press("j", "enter")
		if e.highlighted() != nil {
			t.Errorf("enter left the cursor on %q", e.highlighted())
		}
		e.press("j")
		e.m.cursor = len(e.m.rows) - 1
		if cmd := e.press("enter"); cmd != nil {
			t.Error("enter on the broken space opened something")
		}
		if e.m.remote {
			t.Error("enter on the broken space kept remote navigation going")
		}
	})
}

// Focus puts the cursor on the current session, or on its space when the space is folded.
func TestFocusFindsTheCurrentSession(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.cursor = 0
	e.m.Update(tea.FocusMsg{})
	if got := e.highlighted(); !reflect.DeepEqual(got, []string{"  " + blocked + "dev pr/me/dev"}) {
		t.Errorf("cursor on %q", got)
	}
	snap := fixture(time.Now())
	snap.Folded["/src/gr"] = true
	e.m.Update(snap)
	e.m.Update(tea.BlurMsg{})
	e.m.Update(tea.FocusMsg{})
	if got := e.highlighted(); !reflect.DeepEqual(got, []string{"\u25b6 " + blocked + "gr main"}) {
		t.Errorf("cursor on %q with gr folded", got)
	}
}

// Resting the mouse on a status entry shows it in full over the entries below, until the mouse leaves for
// a row that is not one, or 15 seconds after it last moved. Moving within the popup keeps it, and a click
// on it is one on its entry.
func TestHoverPopup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newSidebarEnv(t)
		e.hover(fmt.Sprintf(entryZone, e.entry(20)))
		_, top := e.zone(popupZone)
		rule := strings.Repeat("\u2500", 29)
		want := []string{blocked + "dev                  +1 5m", "   claude", "   wants permission", "   approve", "   id deploy", "   working build", rule}
		if got := e.screen()[top : top+len(want)]; !reflect.DeepEqual(got, want) {
			t.Errorf("popup:\n%q\nwant\n%q", got, want)
		}

		// The popup covers the Failed entry; moving onto it is moving within the popup.
		time.Sleep(10 * time.Second)
		e.m.Update(tea.MouseMotionMsg{X: 4, Y: top + 5})
		time.Sleep(10 * time.Second)
		e.tick()
		if e.m.hovered() < 0 {
			t.Fatal("the popup closed 10s after the mouse moved within it")
		}
		time.Sleep(6 * time.Second)
		e.tick()
		if e.m.hovered() >= 0 {
			t.Error("the popup stayed 16s after the mouse last moved")
		}

		// Moving onto a spaces row closes it, and so does the mouse leaving the window.
		e.hover(fmt.Sprintf(entryZone, e.entry(20)))
		e.hover(fmt.Sprintf(entryZone, e.row("kitty")))
		if e.m.hovered() >= 0 {
			t.Error("the popup stayed with the mouse on a spaces row")
		}
		e.hover(fmt.Sprintf(entryZone, e.entry(20)))
		e.m.Update(mouseLeftMsg{})
		e.m.render()
		if e.m.hovered() >= 0 {
			t.Error("the popup stayed with the mouse out of the window")
		}

		// An entry whose details its summary already shows has no popup: the failed one's message, the
		// working one's progress, the idle one's program.
		var popups []int
		for _, w := range []int{21, 22, 23} {
			e.hover(fmt.Sprintf(entryZone, e.entry(w)))
			if e.m.hovered() >= 0 {
				popups = append(popups, w)
			}
		}
		if popups != nil {
			t.Errorf("popups for windows %v, which have nothing more to show", popups)
		}

		// A click on the popup opens its entry, not the one it covers.
		e.hover(fmt.Sprintf(entryZone, e.entry(20)))
		_, top = e.zone(popupZone)
		if cmd := e.clickAt(4, top+5, tea.MouseLeft); cmd == nil || e.m.cursor != e.entry(20) || e.m.hovered() >= 0 {
			t.Errorf("click on the popup: opened %v, cursor %d, popup %d", cmd != nil, e.m.cursor, e.m.hovered())
		}
	})
}

// The cursor on a status entry opens it up, as the popup does, with the title and message cut short.
func TestCursorExpandsStatusEntry(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.press("G")
	if got := e.highlighted(); !reflect.DeepEqual(got, []string{"\uf10c  default"}) {
		t.Errorf("cursor on %q", got)
	}
	if !e.has("   claude") {
		t.Error("the idle entry lost its details")
	}
}

// The wheel scrolls the list it is over.
func TestWheel(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.WindowSizeMsg{Width: 30, Height: 20})
	e.m.render()
	e.m.Update(tea.MouseWheelMsg{X: 4, Y: 3, Button: tea.MouseWheelDown})
	e.m.Update(tea.MouseWheelMsg{X: 4, Y: 18, Button: tea.MouseWheelDown})
	if e.m.offset != 1 || e.m.statusOffset != 1 {
		t.Errorf("offsets %d and %d, want 1 and 1", e.m.offset, e.m.statusOffset)
	}
	// The spaces list's first row covers the blank row under default; the status list's indicator takes
	// a row of its own.
	l := e.m.layout()
	got := []string{e.screen()[2], e.screen()[l.statusTop]}
	if want := []string{"\u25b4 1 more", "\u25b4 1 more"}; !reflect.DeepEqual(got, want) {
		t.Errorf("first rows after scrolling = %q, want %q", got, want)
	}
}

// A short sidebar says how many entries each list has out of view, keeps the cursor clear of the
// indicators, and a click on one scrolls a page.
func TestMoreIndicators(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.WindowSizeMsg{Width: 30, Height: 16})
	e.m.render()
	l := e.m.layout()
	spacesRows := func() []string { return e.screen()[headerRows : headerRows+l.spaces] }
	statusRows := func() []string { return e.screen()[l.statusTop : l.statusTop+l.status] }
	want := []string{
		"  " + termIcon + "default",
		"",
		"\u25bc " + repoIcon + "gr main",
		"  " + blocked + "dev pr/me/dev",
		"  " + branchIcon + "fix fix",
		"",
		"\u25be 3 more",
	}
	if got := spacesRows(); !reflect.DeepEqual(got, want) {
		t.Errorf("spaces:\n%q\nwant\n%q", got, want)
	}
	if got := statusRows(); got[len(got)-1] != "\u25be 3 more" {
		t.Errorf("status list's last row = %q:\n%q", got[len(got)-1], got)
	}

	// The cursor down to docs scrolls it into view above the indicator, and one comes at the top.
	e.press("j", "j", "j")
	if got := e.highlighted(); !reflect.DeepEqual(got, []string{"\u25b6 \uf110  docs main"}) {
		t.Fatalf("cursor on %q", got)
	}
	want = []string{
		"\u25b4 3 more",
		"  " + branchIcon + "fix fix",
		"",
		"  " + repoIcon + "kitty master",
		"",
		"\u25b6 \uf110  docs main",
		"\u25be 1 more",
	}
	if got := spacesRows(); !reflect.DeepEqual(got, want) {
		t.Errorf("spaces after j:\n%q\nwant\n%q", got, want)
	}

	// A click on the top indicator scrolls a page up, leaving the cursor.
	e.click(fmt.Sprintf(moreZone, "spaces", -1), tea.MouseLeft)
	if got := spacesRows()[0]; got != "  "+termIcon+"default" || e.m.cursor != e.row("docs") {
		t.Errorf("after clicking the indicator: first row %q, cursor %d", got, e.m.cursor)
	}
	e.click(fmt.Sprintf(moreZone, "status", 1), tea.MouseLeft)
	if got := statusRows()[0]; !strings.HasPrefix(got, "\u25b4 ") {
		t.Errorf("status list's first row after clicking its indicator = %q", got)
	}
}

// A broken config shows on the last line rather than ending the sidebar.
func TestErrorOnLastLine(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(spaces.Snapshot{Err: errors.New("config.toml: unknown keys: space")})
	lines := e.screen()
	if got := lines[len(lines)-1]; got != "config.toml: unknown keys:..." {
		t.Errorf("last line %q", got)
	}
	if !e.has("\u25bc " + repoIcon + "gr main") {
		t.Error("the error dropped what was shown")
	}
}

// In pixel mouse mode, positions are turned into cells, and kitty's event for the mouse leaving the window,
// which bubbletea reads as motion with the left button, into mouseLeftMsg; between a left press and its
// release that motion is a drag.
func TestPixelMouse(t *testing.T) {
	p := &pixelMouse{on: true, cellW: 10, cellH: 20}
	got := []tea.Msg{
		p.filter(nil, tea.MouseMotionMsg{X: 59, Y: 41}),
		p.filter(nil, tea.MouseClickMsg{X: 60, Y: 40, Button: tea.MouseRight}),
		p.filter(nil, tea.MouseWheelMsg{X: 5, Y: 399, Button: tea.MouseWheelDown}),
		p.filter(nil, tea.MouseMotionMsg{X: 3000, Y: 7, Button: tea.MouseLeft}),
		p.filter(nil, tea.KeyPressMsg{Code: 'j', Text: "j"}),
		p.filter(nil, tea.MouseClickMsg{X: 60, Y: 40, Button: tea.MouseLeft}),
		p.filter(nil, tea.MouseMotionMsg{X: 61, Y: 100, Button: tea.MouseLeft}),
		p.filter(nil, tea.MouseReleaseMsg{X: 61, Y: 120, Button: tea.MouseLeft}),
		p.filter(nil, tea.MouseMotionMsg{X: 3000, Y: 7, Button: tea.MouseLeft}),
	}
	want := []tea.Msg{
		tea.MouseMotionMsg{X: 5, Y: 2},
		tea.MouseClickMsg{X: 6, Y: 2, Button: tea.MouseRight},
		tea.MouseWheelMsg{X: 0, Y: 19, Button: tea.MouseWheelDown},
		mouseLeftMsg{},
		tea.KeyPressMsg{Code: 'j', Text: "j"},
		tea.MouseClickMsg{X: 6, Y: 2, Button: tea.MouseLeft},
		tea.MouseMotionMsg{X: 6, Y: 5, Button: tea.MouseLeft},
		tea.MouseReleaseMsg{X: 6, Y: 6, Button: tea.MouseLeft},
		mouseLeftMsg{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filtered:\n%#v\nwant\n%#v", got, want)
	}
	// Off, as before the first mouse event, events pass as they are.
	p.on = false
	if got := p.filter(nil, tea.MouseMotionMsg{X: 3, Y: 4, Button: tea.MouseLeft}); got != (tea.MouseMotionMsg{X: 3, Y: 4, Button: tea.MouseLeft}) {
		t.Errorf("off: %#v", got)
	}
}
