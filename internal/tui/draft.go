package tui

import (
	"maps"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/logs"
	"github.com/chancez/kspaces/internal/state"
	"github.com/chancez/kspaces/internal/tui/interact"
)

// A form typed in keeps what it has when it is left without doing what it is for, by esc, a click
// elsewhere that closes its panel, or its window closing, and has it back when opened again on the same
// thing, see state.Drafts. Clear puts it back as it opened.

// drafts is how a form keeps its draft: key is where in the state, blank what the form opens with
// without one, and done that it did what it is for, which drops the draft.
type drafts struct {
	key   string
	blank map[string]string
	done  bool
}

// drafting is a form that keeps a draft.
type drafting interface {
	popupModel
	drafts() *drafts
	// draft is what the form has, by name, and restore fills it in from one draft returned, leaving what
	// it does not name as it is.
	draft() map[string]string
	restore(map[string]string) tea.Cmd
}

// draftAge is how long a draft is kept.
const draftAge = 7 * 24 * time.Hour

// withDraft is the form build makes, with what the draft kept for key has restored, which build does
// before filling in what the form was asked to open with, so that wins. A form built without one is what
// clear goes back to.
func withDraft[M drafting](key string, build func(saved map[string]string) (M, error)) (M, error) {
	blank, err := build(nil)
	if err != nil {
		return blank, err
	}
	f := blank
	if saved, ok := state.Load().Drafts[key]; ok && time.Since(saved.Saved) < draftAge {
		if f, err = build(saved.Values); err != nil {
			return f, err
		}
	}
	*f.drafts() = drafts{key: key, blank: blank.draft()}
	return f, nil
}

// draftKey is the key of the draft of form opened on on, with inputs given, which are part of how it was
// opened as much as what on.
func draftKey(form string, on []string, inputs map[string]string) string {
	parts := append([]string{form}, on...)
	for _, k := range slices.Sorted(maps.Keys(inputs)) {
		parts = append(parts, k+"="+inputs[k])
	}
	return strings.Join(parts, " ")
}

// changed says whether f has other than what it opens with without a draft, which clear would lose: never
// for a form not opened by withDraft.
func changed(f drafting) bool {
	d := f.drafts()
	return d.blank != nil && !maps.Equal(f.draft(), d.blank)
}

// keepDraft saves what f has as its draft, or drops the draft when f did what it is for or has what it
// opens with, and drops the drafts kept too long. It writes the state only for a change.
func keepDraft(f drafting) {
	d := f.drafts()
	if d.key == "" {
		return
	}
	keep := !d.done && changed(f)
	if _, kept := state.Load().Drafts[d.key]; !keep && !kept {
		return
	}
	values := f.draft()
	err := state.Update(func(s *state.State) {
		maps.DeleteFunc(s.Drafts, func(_ string, saved state.Draft) bool { return time.Since(saved.Saved) >= draftAge })
		if !keep {
			delete(s.Drafts, d.key)
			return
		}
		if s.Drafts == nil {
			s.Drafts = map[string]state.Draft{}
		}
		s.Drafts[d.key] = state.Draft{Values: values, Saved: time.Now()}
	})
	if err != nil {
		logs.Error("saving the form's draft", err)
	}
}

// clearDraft puts a form back as it opens without a draft, while it has something else.
func clearDraft[M drafting, S any]() *interact.Action[M, S] {
	return &interact.Action[M, S]{
		Title:   "clear",
		Applies: func(f M, _ S) bool { return !f.working() && changed(f) },
		Run:     func(f M, _ S) tea.Cmd { return f.restore(f.drafts().blank) },
	}
}
