package tui

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
)

// The move form lists the provider's spaces the name typed matches, but the one the session is in; down
// chooses one, and enter moves there, or to the name as typed, a new space.
func TestMoveSessionsForm(t *testing.T) {
	calls := fakeProvider(t, nil, "")
	s := spaces.ProviderSession{ID: "a1", Name: "kitty.2032"}
	f := newMoveSessionsForm([]string{"prov"}, []spaces.ProviderSession{s}, []string{"default", "gr", "notes", "kitty"})
	f.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	for _, k := range []string{"n", "o"} {
		f.Update(keyMsg(k))
	}
	screen := func(says string) []string {
		return []string{"", "  Move session kitty.2032", "", "  to         no", "             notes", "", "  " + says, "",
			"   Move enter    Cancel esc", ""}
	}
	got := [][]string{formScreen(f)}
	f.Update(keyMsg("down"))
	got = append(got, formScreen(f))
	if want := [][]string{screen("Moves to no, a new space"), screen("Moves to notes")}; !reflect.DeepEqual(got, want) {
		t.Errorf("form:\n%q\nwant\n%q", got, want)
	}
	_, move := f.Update(keyMsg("enter"))
	if _, quit := f.Update(move()); quit == nil {
		t.Fatalf("the form stayed: %v", f.err)
	}
	if got, want := calls(), []string{`move {"session":"a1","space":"notes"}`}; !reflect.DeepEqual(got, want) {
		t.Errorf("asked %q, want %q", got, want)
	}
}
