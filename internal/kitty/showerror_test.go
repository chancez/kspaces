package kitty

import (
	"reflect"
	"testing"
)

// The message reaches kitty as one argument, which it splits as a shell would.
func TestShowError(t *testing.T) {
	calls := fakeKitten(t, "", "")
	if err := ShowError("kspaces", `it's "homelab": $HOME`); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"@", "action", "show_error", "kspaces", `'it'\''s "homelab": $$HOME'`}}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("kitten ran\n%q\nwant\n%q", got, want)
	}
}
