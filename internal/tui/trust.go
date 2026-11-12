package tui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// trustForm shows what a space's own config would add, the commands it would run, and trusts the file's
// content when confirmed. It is meant for a kitty popup, like the forget session form.
type trustForm struct {
	th     *look.Theme
	global config.Config
	space  spaces.Space
	// data is the content shown, which is what trusting trusts, should the file change meanwhile.
	data []byte
	file config.SpaceFile
	// bad is what is wrong with the file, which cannot be trusted until fixed.
	bad   error
	busy  bool
	err   error
	width int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*trustForm]
}

type trustedMsg struct{ err error }

// newTrustForm is the form for trusting data, space's file, with global the config it merges with.
func newTrustForm(global config.Config, space spaces.Space, data []byte) *trustForm {
	f := &trustForm{th: look.New(true), global: global, space: space, data: data}
	f.file, f.bad = config.ParseSpaceFile(string(data))
	if f.bad == nil {
		_, f.bad = global.WithSpace(space.Name, f.file)
	}
	return f
}

func (f *trustForm) Init() tea.Cmd { return nil }

// trust trusts the content shown and leaves.
func (f *trustForm) trust() tea.Cmd {
	f.busy, f.err = true, nil
	path, data := f.space.ConfigPath(), f.data
	return func() tea.Msg { return trustedMsg{spaces.Trust(path, data)} }
}

func (f *trustForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
	case trustedMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theTrustForm.Target(trustSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
	}
	return f, nil
}

func (f *trustForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// adds are lines saying what the file adds, under headings: each command and the line it runs, each
// layout and what each of its panes runs, the defaults and the worktree settings it sets. One the global
// config has too says it takes that one's place.
func (f *trustForm) adds(width int) []string {
	th := f.th
	line := func(spans ...look.Span) string { return th.Render(look.Plain, th.Cut(width, spans...)...) }
	heading := func(s string) string { return line(look.Label(s)) }
	replaces := func(global bool) []look.Span {
		if global {
			return []look.Span{look.Muted("  in place of the global one")}
		}
		return nil
	}
	var out []string
	if cmds := f.file.Commands; len(cmds) > 0 {
		out = append(out, heading("Commands"))
		for _, c := range cmds {
			_, global := f.global.Command(c.Name)
			out = append(out, line(append([]look.Span{look.Text("  " + c.Name + ": " + spaces.JoinWords(c.Run))}, replaces(global)...)...))
		}
	}
	// The panes' commands as the space would have them, its own in place of the global ones.
	merged, _ := f.global.WithSpace(f.space.Name, f.file)
	if ls := f.file.Layouts; len(ls) > 0 {
		out = append(out, heading("Layouts"))
		for _, l := range ls {
			_, global := f.global.Layout(l.Name)
			out = append(out, line(append([]look.Span{look.Text("  " + l.Name)}, replaces(global)...)...))
			for t, tab := range l.Tabs {
				for p, pane := range tab.Panes {
					// Quoted as a shell would split it back, so what runs is what is shown.
					runs := "a shell"
					switch c, ok := merged.PaneCommand(l, pane); {
					case ok && pane.Command != "":
						runs = c.Name + ": " + spaces.JoinWords(c.Run)
					case ok:
						runs = spaces.JoinWords(c.Run)
					}
					out = append(out, line(look.Text(fmt.Sprintf("    tab %d, pane %d: %s", t+1, p+1, runs))))
				}
			}
		}
	}
	var settings []string
	for _, s := range []struct{ key, value string }{
		{"layout", f.file.Layout}, {"worktree_layout", f.file.WorktreeLayout},
	} {
		if s.value != "" {
			settings = append(settings, line(look.Text("  "+s.key+" = "+s.value)))
		}
	}
	for _, s := range []struct {
		key   string
		value *string
	}{{"worktrees.dir", f.file.Dir}, {"worktrees.branch_prefix", f.file.BranchPrefix}} {
		if s.value != nil {
			settings = append(settings, line(look.Text(fmt.Sprintf("  %s = %q", s.key, *s.value))))
		}
	}
	if len(settings) > 0 {
		out = append(append(out, heading("Settings")), settings...)
	}
	if len(out) == 0 {
		out = append(out, line(look.Muted("It adds nothing.")))
	}
	return out
}

func (f *trustForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	text := func(lines []string, s look.Span) []string {
		for _, row := range s.Wrapped(width) {
			lines = append(lines, line(row))
		}
		return lines
	}
	lines := []string{line(look.Heading("Trust " + f.space.Name + "'s config")), ""}
	lines = append(lines, line(th.Cut(width, look.Muted(config.Tilde(f.space.ConfigPath())))...), "")
	if f.bad != nil {
		lines = text(lines, look.Error(f.bad.Error()))
		lines = text(append(lines, ""), look.Text("Fix it, then trust it."))
	} else {
		lines = text(lines, look.Text("Its commands run in "+f.space.Name+"'s worktrees once trusted, until it changes:"))
		lines = append(append(lines, ""), f.adds(width)...)
	}
	lines = append(lines, "")
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Trusting...")), "")
	case f.err != nil:
		lines = append(text(lines, look.Error(f.err.Error())), "")
	}
	f.buttons = controls[*trustForm]{}
	lines = append(lines, line(f.buttons.draw(f, theTrustForm.Target(trustSubject{}),
		control{action: actTrust, label: "Trust", danger: true, off: f.bad != nil},
		control{action: actTrustCancel, label: "Cancel"})...))
	return boxed(lines)
}

// findSpace is the space whose file path is, or that the directory path is in, from any of its
// worktrees, with the config as cfg has it.
func findSpace(cfg config.Config, path string) (spaces.Space, error) {
	dir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}
	top, err := git.TopLevel(dir)
	if err != nil {
		return spaces.Space{}, err
	}
	space, _, err := spaces.FindWorktree(cfg, top)
	return space, err
}

// RunTrust runs the trust form for the space file of the space path is in, or path names, meant for a
// kitty popup. With yes it trusts the file as it is without asking; with revoke it forgets it.
func RunTrust(ctx context.Context, cfg config.Config, path string, yes, revoke bool) error {
	space, err := findSpace(cfg, path)
	if err != nil {
		return err
	}
	file := space.ConfigPath()
	if revoke {
		return spaces.Revoke(file)
	}
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s has no %s", space.Name, config.SpaceFileName)
	}
	if err != nil {
		return err
	}
	f := newTrustForm(cfg, space, data)
	if yes {
		if f.bad != nil {
			return fmt.Errorf("%s: %w", config.Tilde(file), f.bad)
		}
		return spaces.Trust(file, data)
	}
	if err := runPopup(ctx, f); err != nil {
		return fmt.Errorf("running the trust form: %w", err)
	}
	return nil
}
