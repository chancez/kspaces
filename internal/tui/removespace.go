package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
	"github.com/chancez/kspaces/internal/tui/look"
)

// removeSpaceForm shows what removing a space does, see spaces.RemoveSpace, and starts it when confirmed:
// for a configured space taking it out of the config, its repository left alone; for a topic space
// removing the provider's space; either way closing its kitty sessions and forgetting their files, and
// doing what is chosen to the provider's sessions in it. It is meant for a kitty popup, like the remove
// worktree form.
type removeSpaceForm struct {
	th    *look.Theme
	plan  spaces.SpaceRemoval
	fate  fateChoice
	busy  bool
	err   error
	width int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*removeSpaceForm]
}

type spaceRemovedMsg struct{ err error }

// newRemoveSpaceForm is the form for plan.
func newRemoveSpaceForm(plan spaces.SpaceRemoval) *removeSpaceForm {
	return &removeSpaceForm{th: look.New(true), plan: plan, fate: fateChoice{fates: plan.Fates}}
}

func (f *removeSpaceForm) Init() tea.Cmd { return nil }

// refused is why the space cannot be removed: a topic space the provider cannot remove, or not end the
// sessions of.
func (f *removeSpaceForm) refused() error {
	if f.plan.Root == "" && len(f.plan.Fates) == 0 {
		return fmt.Errorf("the session provider cannot remove %s, or end the sessions in it", f.plan.Name)
	}
	return nil
}

// fateChosen is what becomes of the provider's sessions: the choice, or for a topic space ending them, the
// one way it goes.
func (f *removeSpaceForm) fateChosen() spaces.Fate {
	if f.plan.Root == "" {
		return spaces.FateEnd
	}
	return f.fate.chosen()
}

// remove hands the removal to a background process and leaves: removing closes the space's windows, and
// this form may be over one of them.
func (f *removeSpaceForm) remove() tea.Cmd {
	f.busy, f.err = true, nil
	name, fate := f.plan.Ref, f.fateChosen()
	return func() tea.Msg { return spaceRemovedMsg{spaces.RemoveSpaceInBackground(name, fate)} }
}

func (f *removeSpaceForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
	case spaceRemovedMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theRemoveSpaceForm.Target(removeSpaceSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
		if f.fate.click(f.zones, msg) {
			return f, nil
		}
	}
	return f, nil
}

func (f *removeSpaceForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (f *removeSpaceForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	heading := "Remove space " + f.plan.Name
	if f.plan.Host != "" {
		heading += " on " + f.plan.Host
	}
	lines := []string{line(look.Heading(heading)), ""}
	text := func(s string) {
		for _, row := range look.Text(s).Wrapped(width) {
			lines = append(lines, line(row))
		}
	}
	if f.plan.Root != "" {
		text(fmt.Sprintf("Takes %s out of the spaces in the config. The repository stays as it is.", config.Tilde(f.plan.Root)))
	} else if f.plan.Host != "" {
		text("Has " + f.plan.Host + " end the sessions in " + f.plan.Name + " there, and remove the space.")
	} else if f.plan.AdHoc {
		text("Removes the ad hoc space " + f.plan.Name + ", ending the sessions in it.")
	} else {
		text("Removes the session provider's space " + f.plan.Name + ".")
	}
	if n := len(f.plan.Sessions); n > 0 {
		text(fmt.Sprintf("Closes %s and deletes the saved files: %s.", plural(n, "kitty session"), strings.Join(f.plan.Sessions, ", ")))
	}
	lines = append(lines, "")
	if f.fate.shown() {
		lines = append(append(lines, f.fate.lines(th, width, f.plan.Provider)...), "")
	}
	err := f.err
	if err == nil {
		err = f.refused()
	}
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Removing...")), "")
	case err != nil:
		for _, row := range look.Error(err.Error()).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*removeSpaceForm]{}
	lines = append(lines, line(f.buttons.draw(f, theRemoveSpaceForm.Target(removeSpaceSubject{}),
		control{action: actRemoveSpace, label: "Remove", danger: true, off: !f.busy && f.refused() != nil},
		control{action: actRemoveSpaceCancel, label: "Cancel"})...))
	return boxed(lines)
}

func (f *removeSpaceForm) theme() *look.Theme { return f.th }
func (f *removeSpaceForm) working() bool      { return f.busy }
func (f *removeSpaceForm) failed() bool       { return f.err != nil }
func (f *removeSpaceForm) keys() ([]interact.Section, bool) {
	return interact.Help(f, theRemoveSpaceForm.Target(removeSpaceSubject{})), false
}

// RunRemoveSpace runs the remove space form for the space name, meant for a kitty popup; with yes it
// removes it without asking, fate saying what becomes of the provider's sessions in it, the first the
// provider allows when it allows none named.
func RunRemoveSpace(ctx context.Context, cfg config.Config, name string, yes bool, fate spaces.Fate) error {
	plan, err := spaces.PlanSpaceRemoval(cfg, name)
	if err != nil {
		return err
	}
	f := newRemoveSpaceForm(plan)
	if yes {
		if err := f.refused(); err != nil {
			return err
		}
		chosen := f.fateChosen()
		if fate != "" && plan.Root != "" {
			chosen = fate
		}
		return spaces.RemoveSpace(cfg, name, chosen)
	}
	if err := runPopup(ctx, f); err != nil {
		return fmt.Errorf("running the remove space form: %w", err)
	}
	return nil
}
