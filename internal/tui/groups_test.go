package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// groupsFixture is fixture with gr and docs in work and kitty in personal, which groups lists in that
// order, and broken in none.
func groupsFixture() spaces.Snapshot {
	snap := fixture(time.Now())
	cfg := *snap.Config
	cfg.Spaces = []string{"/src/gr", "/src/kitty", "/src/docs", "/src/broken"}
	cfg.Space = map[string]config.SpaceSettings{"gr": {Group: "work"}, "docs": {Group: "work"}, "kitty": {Group: "personal"}}
	cfg.Groups = []string{"work", "personal"}
	snap.Config = &cfg
	for i := range snap.Spaces {
		snap.Spaces[i].Group = cfg.Space[snap.Spaces[i].Name].Group
	}
	snap.Order.Groups = cfg.Groups
	return snap
}

// newGroupsEnv is newSidebarEnv showing groupsFixture.
func newGroupsEnv(t *testing.T) *sidebarEnv {
	t.Helper()
	e := newSidebarEnv(t)
	e.m.Update(groupsFixture())
	e.m.render()
	return e
}

// stateFile is what the state file name says.
func stateFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(os.Getenv("XDG_STATE_HOME"), "kspaces", name))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

// The rows of groupsFixture, by what they show.
var (
	groupGr     = []string{"\u25bc " + repoIcon + "gr main", here + blocked + "dev pr/me/dev", "  " + branchIcon + "fix fix"}
	groupDocs   = "\u25b6 \uf110  docs main"
	groupKitty  = "  " + repoIcon + "kitty master"
	groupBroken = "  \uf00d  broken"
	workHeader  = headerLine(false, "WORK", 30)
	homeHeader  = headerLine(false, "PERSONAL", 30)
	workFolded  = heldHeaderLine(true, "WORK", 30)
)

// headerLine is a group's header in a list width cells wide: a rule in the caret column, its name, its
// fold icon, an open or a closed folder, and the rule on to the edge.
func headerLine(folded bool, name string, width int) string {
	icon := "\uf07c"
	if folded {
		icon = "\uf07b"
	}
	return "\u2500\u2500 " + name + " " + icon + "  " + strings.Repeat("\u2500", width-3-len(name)-4)
}

// heldHeaderLine is headerLine for a folded group holding the sidebar's own session, gr@dev: the edge of
// the session's row in place of the rule's start.
func heldHeaderLine(folded bool, name string, width int) string {
	return here + strings.TrimPrefix(headerLine(folded, name, width), "\u2500\u2500")
}

// A header is a divider: a rule, its name, its fold icon and the rule to the edge, at the default width
// and at the narrowest, where the rule after the icon goes first and then the name is cut, the icon kept,
// folded or not.
func TestGroupHeaderWidths(t *testing.T) {
	wide := func(rows ...string) []string {
		return append([]string{"SPACES                 \uf0b0 \uf0c9 \uf067", "", "  " + termIcon + "default", "", groupBroken, ""}, rows...)
	}
	narrow := func(rows ...string) []string {
		return append([]string{"SPACES \uf0b0 \uf0c9 \uf067", "", "  " + termIcon + "default", "", groupBroken, ""}, rows...)
	}
	grNarrow := []string{"\u25bc " + repoIcon + "gr main", here + blocked + "dev pr\u2026", "  " + branchIcon + "fix fix", "", "\u25b6 \uf110  docs m\u2026"}
	for _, c := range []struct {
		width  int
		folded bool
		want   []string
	}{
		{config.DefaultSidebarWidth, false, wide(headerLine(false, "WORK", 28), groupGr[0], groupGr[1], groupGr[2], "", groupDocs, "", headerLine(false, "PERSONAL", 28), groupKitty)},
		{config.DefaultSidebarWidth, true, wide(heldHeaderLine(true, "WORK", 28), "", headerLine(true, "PERSONAL", 28))},
		{config.MinSidebarWidth, false, narrow(append(append([]string{"\u2500\u2500 WORK \uf07c  \u2500"}, grNarrow...), "", "\u2500\u2500 PERS\u2026 \uf07c", "  "+repoIcon+"kitty")...)},
		{config.MinSidebarWidth, true, narrow(here+" WORK \uf07b  \u2500", "", "\u2500\u2500 PERS\u2026 \uf07b")},
	} {
		e := newGroupsEnv(t)
		e.m.Update(tea.WindowSizeMsg{Width: c.width, Height: 40})
		snap := groupsFixture()
		if c.folded {
			snap.Folded = map[string]bool{"/src/docs": true, spaces.GroupKey("work"): true, spaces.GroupKey("personal"): true}
		}
		e.m.Update(snap)
		if got := e.spacesScreen(); !reflect.DeepEqual(got, c.want) {
			t.Errorf("width %d, folded %v:\n%q\nwant\n%q", c.width, c.folded, got, c.want)
		}
	}

	// Filtering, which ignores folds, has no icon: the rule runs on after the name. k is in broken too.
	e := newGroupsEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.press("/", "k")
	want := []string{spacesTitle, filterPrompt + "k", groupBroken, "", "\u2500\u2500 PERSONAL " + strings.Repeat("\u2500", 30-12), groupKitty}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("filtering:\n%q\nwant\n%q", got, want)
	}
}

// groupsScreen is the spaces list with rows after the session in no space and broken, in no group.
func groupsScreen(rows ...string) []string {
	return append([]string{spacesTitle, "", "  " + termIcon + "default", "", groupBroken}, rows...)
}

// The spaces in no group come first, then each group under a header, muted, with a caret, its spaces
// under it.
func TestGroupHeaders(t *testing.T) {
	e := newGroupsEnv(t)
	want := groupsScreen("", workHeader, groupGr[0], groupGr[1], groupGr[2], "", groupDocs, "", homeHeader, groupKitty)
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("spaces list:\n%q\nwant\n%q", got, want)
	}
	line := strings.Split(e.m.render(), "\n")[headerRows+e.headerRow("work")]
	if name := e.m.th.Render(look.Plain, look.GroupHeader("work")); !strings.Contains(line, name) {
		t.Errorf("header %q has no %q", line, name)
	}
}

// headerRow is the index of the header of group.
func (e *sidebarEnv) headerRow(group string) int {
	e.t.Helper()
	for i, r := range e.m.rows {
		if r.Group != nil && r.Group.Name == group {
			return i
		}
	}
	e.t.Fatalf("no header for %s", group)
	return 0
}

// space on a group's header folds its spaces into it, in every sidebar, and again shows them.
func TestFoldGroupKey(t *testing.T) {
	e := newGroupsEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.press("k", "k")
	if got := e.highlighted(); !reflect.DeepEqual(got, []string{workHeader}) {
		t.Fatalf("cursor on %q", got)
	}
	e.press("space")
	type state struct {
		Screen, Cursor []string
		Folded         string
	}
	got := state{e.spacesScreen(), e.highlighted(), e.folded()}
	want := state{groupsScreen("", workFolded, "", homeHeader, groupKitty), []string{workFolded}, "/src/docs\ngroup:work\n"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("folded:\n%#v\nwant\n%#v", got, want)
	}
	e.press("space")
	got = state{e.spacesScreen(), e.highlighted(), e.folded()}
	want = state{groupsScreen("", workHeader, groupGr[0], groupGr[1], groupGr[2], "", groupDocs, "", homeHeader, groupKitty), []string{workHeader}, "/src/docs\n"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unfolded:\n%#v\nwant\n%#v", got, want)
	}
}

// A click anywhere on a header folds its group or shows it again: its fold icon, its name or its rule,
// as a header has nothing to open. Its menu folds it too.
func TestFoldGroupClick(t *testing.T) {
	e := newGroupsEnv(t)
	work := e.headerRow("work")
	folded := groupsScreen("", workFolded, "", homeHeader, groupKitty)
	unfolded := groupsScreen("", workHeader, groupGr[0], groupGr[1], groupGr[2], "", groupDocs, "", homeHeader, groupKitty)
	x, y := e.zone(fmt.Sprintf(entryZone, work))
	for _, c := range []struct {
		on     string
		click  func()
		screen []string
		saved  string
	}{
		{"icon", func() { e.click(fmt.Sprintf(caretZone, work), tea.MouseLeft) }, folded, "/src/docs\ngroup:work\n"},
		{"name", func() { e.clickAt(x+3, y, tea.MouseLeft) }, unfolded, "/src/docs\n"},
		{"leading rule", func() { e.clickAt(x, y, tea.MouseLeft) }, folded, "/src/docs\ngroup:work\n"},
		{"trailing rule", func() { e.clickAt(x+20, y, tea.MouseLeft) }, unfolded, "/src/docs\n"},
	} {
		c.click()
		if got := e.spacesScreen(); !reflect.DeepEqual(got, c.screen) || e.folded() != c.saved {
			t.Errorf("%s clicked, folded %q:\n%q\nwant\n%q", c.on, e.folded(), got, c.screen)
		}
	}

	e.click(fmt.Sprintf(entryZone, work), tea.MouseRight)
	want := []string{menuRule("Group"), " Fold group", menuRule("Space"), " Select", " Add space", menuRule("")}
	if got := e.screen()[y+1 : y+7]; !reflect.DeepEqual(got, want) {
		t.Errorf("menu:\n%q\nwant\n%q", got, want)
	}
	e.click(fmt.Sprintf(menuZone, 0), tea.MouseLeft)
	if got := e.spacesScreen(); !reflect.DeepEqual(got, folded) {
		t.Errorf("Fold group:\n%q\nwant\n%q", got, folded)
	}
}

// The list's menu has a Groups section, each group ticked while it is shown; choosing one hides it, in
// every sidebar, header, spaces and status entries all, and choosing it again shows it. The attention
// filter shows nothing of a hidden group.
func TestGroupsMenu(t *testing.T) {
	e := newGroupsEnv(t)
	menu := func(work, personal bool) []string {
		return []string{
			menuRule("Sort"), "\uf00c Manual", "  Activity", "  Status",
			menuRule("Filter"), "  Only what is not idle",
			menuRule("Rows"), "  Select",
			menuRule("Groups"), mark(work) + " work", mark(personal) + " personal",
			menuRule(""),
		}
	}
	e.click(sortZone, tea.MouseRight)
	if got := e.screen()[1:13]; !reflect.DeepEqual(got, menu(true, true)) {
		t.Errorf("menu:\n%q\nwant\n%q", got, menu(true, true))
	}
	type state struct {
		Screen  []string
		Windows []int
		Hidden  string
	}
	now := func() state {
		var windows []int
		for _, s := range e.m.kitty.Statuses {
			windows = append(windows, s.Window)
		}
		return state{e.spacesScreen(), windows, stateFile(t, "hidden")}
	}
	e.click(fmt.Sprintf(menuZone, 5), tea.MouseLeft)
	hidden := state{groupsScreen("", homeHeader, groupKitty), []int{21, 23}, "work\n"}
	if got := now(); e.m.menu != nil || !reflect.DeepEqual(got, hidden) {
		t.Errorf("work hidden, menu open %v:\n%#v\nwant\n%#v", e.m.menu != nil, got, hidden)
	}
	e.click(sortZone, tea.MouseRight)
	if got := e.screen()[1:13]; !reflect.DeepEqual(got, menu(false, true)) {
		t.Errorf("menu with work hidden:\n%q\nwant\n%q", got, menu(false, true))
	}
	e.press("esc")

	// Of what is not idle, only work's sessions and a window in no session have a status.
	e.press("f")
	nothing := []string{spacesTitle, "", "Nothing needs you"}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, nothing) {
		t.Errorf("attention filter:\n%q\nwant\n%q", got, nothing)
	}
	e.press("f")

	// A refresh keeps it hidden, as every sidebar reads it.
	snap := groupsFixture()
	snap.Hidden = spaces.LoadHidden()
	e.m.Update(snap)
	if got := now(); !reflect.DeepEqual(got, hidden) {
		t.Errorf("refreshed:\n%#v\nwant\n%#v", got, hidden)
	}

	e.click(sortZone, tea.MouseRight)
	e.click(fmt.Sprintf(menuZone, 5), tea.MouseLeft)
	shown := state{groupsScreen("", workHeader, groupGr[0], groupGr[1], groupGr[2], "", groupDocs, "", homeHeader, groupKitty), []int{20, 21, 22, 23}, ""}
	if got := now(); !reflect.DeepEqual(got, shown) {
		t.Errorf("work shown:\n%#v\nwant\n%#v", got, shown)
	}
}

// mark is the mark of a menu item chosen, or the blank of one not.
func mark(chosen bool) string {
	if chosen {
		return "\uf00c"
	}
	return " "
}

// K and J move a space among its group's only: past the first or the last of them they do nothing, and a
// space alone in its group, or in none, stays put.
func TestMoveKeysInGroup(t *testing.T) {
	e := newGroupsEnv(t)
	cfg := newConfig(t, orderConfig)
	type state struct {
		Screen, Cursor []string
		Config         string
	}
	now := func() state { return state{e.spacesScreen(), e.highlighted(), cfg()} }
	unmoved := state{groupsScreen("", workHeader, groupGr[0], groupGr[1], groupGr[2], "", groupDocs, "", homeHeader, groupKitty), []string{groupGr[0]}, orderConfig}

	e.m.Update(tea.FocusMsg{})
	e.press("k", "K")
	if got := now(); !reflect.DeepEqual(got, unmoved) {
		t.Errorf("K on gr, first in work:\n%#v\nwant\n%#v", got, unmoved)
	}
	e.press("J")
	moved := state{groupsScreen("", workHeader, groupDocs, "", groupGr[0], groupGr[1], groupGr[2], "", homeHeader, groupKitty), []string{groupGr[0]},
		"# Mine.\nspaces = [\n  \"/src/broken\",\n  \"/src/docs\",\n  \"/src/gr\", # work\n  # the terminal\n  \"/src/kitty\",\n]\n"}
	if got := now(); !reflect.DeepEqual(got, moved) {
		t.Errorf("J on gr:\n%#v\nwant\n%#v", got, moved)
	}
	e.press("J")
	if got := now(); !reflect.DeepEqual(got, moved) {
		t.Errorf("J on gr, last in work:\n%#v\nwant\n%#v", got, moved)
	}
	for _, c := range []struct {
		name string
		keys []string
		on   string
	}{
		// From gr@dev, the sidebar's own, where focus puts the cursor.
		{"kitty, alone in personal", []string{"j", "j", "j", "K", "J"}, groupKitty},
		{"broken, alone in no group", []string{"g", "j", "K", "J"}, groupBroken},
	} {
		e.m.Update(tea.FocusMsg{})
		e.press(c.keys...)
		want := moved
		want.Cursor = []string{c.on}
		if got := now(); !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n%#v\nwant\n%#v", c.name, got, want)
		}
	}
}

// Dragging a space past its group's spaces leaves it first or last among them, never in another group.
func TestDragInGroup(t *testing.T) {
	e := newGroupsEnv(t)
	cfg := newConfig(t, orderConfig)
	type state struct {
		Screen []string
		Config string
	}
	now := func() state { return state{e.spacesScreen(), cfg()} }
	// kitty onto docs, in work: personal has kitty alone, so nothing moves.
	e.pressOn("kitty", headerRows+e.row("docs"))
	e.letGo(headerRows + e.row("docs"))
	unmoved := state{groupsScreen("", workHeader, groupGr[0], groupGr[1], groupGr[2], "", groupDocs, "", homeHeader, groupKitty), orderConfig}
	if got := now(); !reflect.DeepEqual(got, unmoved) {
		t.Errorf("kitty onto docs:\n%#v\nwant\n%#v", got, unmoved)
	}
	// gr onto kitty, below work: gr goes last in work.
	e.pressOn("gr", headerRows+e.row("kitty"))
	e.letGo(headerRows + e.row("kitty"))
	moved := state{groupsScreen("", workHeader, groupDocs, "", groupGr[0], groupGr[1], groupGr[2], "", homeHeader, groupKitty),
		"# Mine.\nspaces = [\n  \"/src/broken\",\n  \"/src/docs\",\n  \"/src/gr\", # work\n  # the terminal\n  \"/src/kitty\",\n]\n"}
	if got := now(); !reflect.DeepEqual(got, moved) {
		t.Errorf("gr onto kitty:\n%#v\nwant\n%#v", got, moved)
	}
}

// The picker names a grouped space's entries after its group, so a query finds them by it.
func TestPickerGroups(t *testing.T) {
	m := newPicker(0)
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 40})
	m.Update(groupsFixture())
	got := pickerScreen(m)[6:13]
	want := []string{
		open + "default",
		open + "work / gr  main",
		blocked + "work / gr / dev  pr/me/dev",
		"   work / gr / fix  fix",
		"   work / docs  main",
		"\uf110  work / docs / notes  notes",
		"   personal / kitty  master",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("picker:\n%q\nwant\n%q", got, want)
	}
	typeInto(m, "personal")
	if got := pickerScreen(m)[2:4]; !reflect.DeepEqual(got, []string{"   personal / kitty  master", ""}) {
		t.Errorf("after personal:\n%q", got)
	}
}
