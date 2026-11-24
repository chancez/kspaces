package tui

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
)

// The remove space form says what goes: a configured space from the config, its repository kept, or a
// topic space from the provider, its kitty sessions, and the provider's sessions as chosen; a topic space
// the provider cannot remove is refused.
func TestRemoveSpaceForm(t *testing.T) {
	sessions := []spaces.ProviderSession{{ID: "a", Name: "build", State: "running", Busy: true, Command: "make"}}
	configured := newRemoveSpaceForm(spaces.SpaceRemoval{Name: "gr", Root: "/src/gr", Sessions: []string{"gr", "gr@fix"}, Provider: sessions,
		Fates: []spaces.Fate{spaces.FateEnd, spaces.FateShelve, spaces.FateLeave}})
	topic := newRemoveSpaceForm(spaces.SpaceRemoval{Name: "notes", Sessions: []string{"notes"}, Provider: sessions, Fates: []spaces.Fate{spaces.FateEnd}})
	refused := newRemoveSpaceForm(spaces.SpaceRemoval{Name: "notes"})
	var got [][]string
	for _, f := range []*removeSpaceForm{configured, topic, refused} {
		f.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
		got = append(got, formScreen(f))
	}
	want := [][]string{
		{"", "  Remove space gr", "", "  Takes /src/gr out of the spaces in the config. The repository stays as it is.",
			"  Closes 2 kitty sessions and deletes the saved files: gr, gr@fix.", "",
			"  sessions   < end           >  left/right change", "             Ends 1 session: build running make", "",
			"   Remove y    Cancel n", ""},
		{"", "  Remove space notes", "", "  Removes the session provider's space notes.",
			"  Closes 1 kitty session and deletes the saved files: notes.", "",
			"  sessions   end", "             Ends 1 session: build running make", "",
			"   Remove y    Cancel n", ""},
		{"", "  Remove space notes", "", "  Removes the session provider's space notes.", "",
			"  the session provider cannot remove notes, or end the sessions in it", "",
			"   Remove    Cancel n", ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
	if _, cmd := refused.Update(keyMsg("y")); cmd != nil {
		t.Error("y removed a space the provider cannot remove")
	}
}
