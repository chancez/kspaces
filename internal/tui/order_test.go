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

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// orderFixture is fixture with focus times, gr@fix focused last, then kitty, docs@notes, gr@dev and
// default, gr and broken never, and the sort mode saved, as spaces.Shared.View reads it.
func orderFixture() spaces.Snapshot {
	snap := fixture(time.Now())
	snap.Order = spaces.Order{
		Mode:     spaces.LoadSort(),
		Focused:  map[string]float64{"gr@fix": 50, "kitty": 30, "docs@notes": 20, "gr@dev": 10, "default": 5},
		Statuses: snap.Kitty.Statuses,
	}
	return snap
}

// newOrderEnv is newSidebarEnv showing orderFixture.
func newOrderEnv(t *testing.T) *sidebarEnv {
	t.Helper()
	e := newSidebarEnv(t)
	e.m.Update(orderFixture())
	e.m.render()
	return e
}

// savedSort is what the sort file says.
func savedSort(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(os.Getenv("XDG_STATE_HOME"), "kspaces", "sort"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

// The spaces list in each sort mode, its button on the title showing the mode.
var (
	manualScreen = []string{
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
	// gr by fix, the most recent, then kitty and docs; broken never focused.
	activityScreen = []string{
		"SPACES                    \uf017 \uf067",
		"",
		"  " + termIcon + "default",
		"",
		"\u25bc " + repoIcon + "gr main",
		"  " + branchIcon + "fix fix",
		"  " + blocked + "dev pr/me/dev",
		"",
		"  " + repoIcon + "kitty master",
		"",
		"\u25b6 \uf110  docs main",
		"",
		"  \uf00d  broken",
	}
	// gr by dev, blocked, then docs by notes, working; kitty and broken have none, kitty focused later.
	statusScreen = []string{
		"SPACES                    \uf071 \uf067",
		"",
		"  " + termIcon + "default",
		"",
		"\u25bc " + repoIcon + "gr main",
		"  " + blocked + "dev pr/me/dev",
		"  " + branchIcon + "fix fix",
		"",
		"\u25b6 \uf110  docs main",
		"",
		"  " + repoIcon + "kitty master",
		"",
		"  \uf00d  broken",
	}
)

// A click on the sort button, or s, goes on to the next mode, in every sidebar: the rows reorder and the
// cursor stays on its row. The button is lit while the mouse is on it.
func TestSortButton(t *testing.T) {
	e := newOrderEnv(t)
	e.m.Update(tea.FocusMsg{})
	if got := e.spacesScreen(); !reflect.DeepEqual(got, manualScreen) {
		t.Fatalf("manual:\n%q\nwant\n%q", got, manualScreen)
	}
	lit := e.m.th.Render(look.Plain, look.SortButton(spaces.SortManual, true))
	e.hover(sortZone)
	if got := strings.Split(e.m.render(), "\n")[0]; !strings.Contains(got, lit) {
		t.Errorf("button under the mouse not lit: %q", got)
	}
	e.m.Update(mouseLeftMsg{})
	if got := strings.Split(e.m.render(), "\n")[0]; strings.Contains(got, lit) {
		t.Errorf("button lit with the mouse gone: %q", got)
	}

	for _, step := range []struct {
		name   string
		cycle  func()
		screen []string
		saved  string
	}{
		{"click", func() { e.click(sortZone, tea.MouseLeft) }, activityScreen, "activity\n"},
		{"s", func() { e.press("s") }, statusScreen, "status\n"},
		{"click", func() { e.click(sortZone, tea.MouseLeft) }, manualScreen, "manual\n"},
	} {
		step.cycle()
		if got := e.spacesScreen(); !reflect.DeepEqual(got, step.screen) {
			t.Errorf("after a %s:\n%q\nwant\n%q", step.name, got, step.screen)
		}
		if got := savedSort(t); got != step.saved {
			t.Errorf("after a %s saved %q, want %q", step.name, got, step.saved)
		}
		if got := e.highlighted(); !reflect.DeepEqual(got, []string{"  " + blocked + "dev pr/me/dev"}) {
			t.Errorf("after a %s the cursor is on %q", step.name, got)
		}
	}

	// A refresh keeps the mode and the cursor's row as it reorders.
	e.press("s")
	e.m.Update(orderFixture())
	if got := e.spacesScreen(); !reflect.DeepEqual(got[4:7], activityScreen[4:7]) {
		t.Errorf("refreshed:\n%q", got)
	}
	if got := e.highlighted(); !reflect.DeepEqual(got, []string{"  " + blocked + "dev pr/me/dev"}) {
		t.Errorf("refreshed, the cursor is on %q", got)
	}
}

// A right click on the sort button opens a menu of the modes under it, the one chosen now marked and
// selected; choosing one sorts by it.
func TestSortMenu(t *testing.T) {
	e := newOrderEnv(t)
	e.click(sortZone, tea.MouseRight)
	want := []string{menuRule(""), "\uf00c Manual", "  Activity", "  Status", menuRule("")}
	if got := e.screen()[1:6]; !reflect.DeepEqual(got, want) {
		t.Errorf("menu:\n%q\nwant\n%q", got, want)
	}
	if got := e.highlighted(); !reflect.DeepEqual(got, []string{"\uf00c Manual"}) {
		t.Errorf("highlighted %q", got)
	}
	e.click(fmt.Sprintf(menuZone, 2), tea.MouseLeft)
	if got := e.spacesScreen(); e.m.menu != nil || !reflect.DeepEqual(got, statusScreen) {
		t.Errorf("Status chose, menu open %v:\n%q\nwant\n%q", e.m.menu != nil, got, statusScreen)
	}
	if got := savedSort(t); got != "status\n" {
		t.Errorf("saved %q", got)
	}
	e.click(sortZone, tea.MouseRight)
	want = []string{menuRule(""), "  Manual", "  Activity", "\uf00c Status", menuRule("")}
	if got := e.screen()[1:6]; !reflect.DeepEqual(got, want) {
		t.Errorf("menu again:\n%q\nwant\n%q", got, want)
	}
}
