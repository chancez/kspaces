package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// With a provider that can end its sessions, closing says how many of them it ends, and left and right
// choose to suspend them, kept to open again, or keep them running, the form's words following.
func TestCloseFormSessions(t *testing.T) {
	f := &closeSessionForm{th: look.New(true), cfg: config.Config{Provider: []string{"prov"}}, mode: spaces.KeepClosed,
		sessions: []string{"gr@dev"}, windows: []int{2},
		closing: map[string][]spaces.ProviderSession{"gr@dev": {{ID: "a", Name: "build", State: "running", Busy: true, Command: "make"}}},
		fate:    fateChoice{fates: []spaces.Fate{spaces.FateEnd, spaces.FateShelve, spaces.FateLeave}}}
	f.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	screen := func(does, choice, says string) []string {
		return append(append([]string{"", "  Close session gr@dev", "", "  sessions   < " + choice + " >  left/right change", "             " + says, ""},
			strings.Split("  "+does, "\n")...), "", "   Close y    Cancel n", "")
	}
	var got [][]string
	for range 3 {
		got = append(got, formScreen(f))
		f.Update(keyMsg("right"))
	}
	want := [][]string{
		screen("Closes its 2 windows, and what runs in them. Opening it again restores its layout, with fresh\n  programs.", "end         ", "Ends 1 session: build running make"),
		screen("Stops what runs in its 2 windows and closes them. Opening it again brings them back as they\n  were.", "suspend     ", "Shelves 1 session, to bring back later: build running make"),
		screen("Closes 2 windows, leaving what runs in them running.", "keep running", "Leaves 1 session running"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("form:\n%q\nwant\n%q", got, want)
	}
}

// A click on the choice's arrows goes to the fate before or after, as left and right do.
func TestCloseFormFateClick(t *testing.T) {
	f := &closeSessionForm{th: look.New(true), cfg: config.Config{Provider: []string{"prov"}}, mode: spaces.KeepClosed,
		sessions: []string{"gr@dev"}, windows: []int{1},
		fate: fateChoice{fates: []spaces.Fate{spaces.FateEnd, spaces.FateShelve, spaces.FateLeave}}}
	f.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	click := func(zone string) spaces.Fate {
		f.View()
		z := f.zones[zone][0]
		f.Update(tea.MouseClickMsg{X: z.X, Y: z.Y, Button: tea.MouseLeft})
		return f.fate.chosen()
	}
	got := []spaces.Fate{click(fateNextZone), click(fateNextZone), click(fatePrevZone), click(fatePrevZone), click(fatePrevZone)}
	want := []spaces.Fate{spaces.FateShelve, spaces.FateLeave, spaces.FateShelve, spaces.FateEnd, spaces.FateLeave}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// The choice's arrows sit in the same cells whichever fate is chosen, for one session and for several, so
// a click on an arrow leaves the mouse on it: the words that change go below the choice, and each name is
// padded to the widest.
func TestCloseFormFateStill(t *testing.T) {
	one := &closeSessionForm{th: look.New(true), cfg: config.Config{Provider: []string{"prov"}}, mode: spaces.KeepClosed,
		sessions: []string{"gr@dev"}, windows: []int{2},
		closing: map[string][]spaces.ProviderSession{"gr@dev": {{ID: "a", Name: "build", State: "running", Busy: true, Command: "make"}}},
		fate:    fateChoice{fates: []spaces.Fate{spaces.FateEnd, spaces.FateShelve, spaces.FateLeave}}}
	several := &closeSessionForm{th: look.New(true), cfg: config.Config{Provider: []string{"prov"}}, mode: spaces.KeepClosed,
		sessions: []string{"gr@dev", "gr@fix"}, windows: []int{2, 1},
		closing: map[string][]spaces.ProviderSession{"gr@dev": {{ID: "a", Name: "build", State: "running", Busy: true, Command: "make"}}},
		fate:    fateChoice{fates: []spaces.Fate{spaces.FateEnd, spaces.FateShelve, spaces.FateLeave}}}
	for _, f := range []*closeSessionForm{one, several} {
		f.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
		var got [][]look.Zone
		for range 3 {
			f.View()
			got = append(got, []look.Zone{f.zones[fatePrevZone][0], f.zones[fateNextZone][0]})
			f.Update(keyMsg("right"))
		}
		if want := [][]look.Zone{got[0], got[0], got[0]}; !reflect.DeepEqual(got, want) {
			t.Errorf("%v: arrows moved: %v", f.sessions, got)
		}
	}
}

// A fate alone is said as it is, with no arrows to change it, and no key does.
func TestFateAlone(t *testing.T) {
	c := fateChoice{fates: []spaces.Fate{spaces.FateEnd}}
	got := c.lines(look.New(true), 80, []spaces.ProviderSession{{Name: "a", State: "exited"}})
	for i := range got {
		got[i] = strings.TrimRight(ansi.Strip(got[i]), " ")
	}
	if want := []string{"sessions   end", "           Ends 1 session"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if c.cycle(1); c.chosen() != "" || c.at != 0 {
		t.Errorf("a fate alone changed: %v", c)
	}
}
