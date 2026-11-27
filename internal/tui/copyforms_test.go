package tui

import (
	"errors"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

// pressKey sends the key s names, see keyMsg, and is the command it returns.
func pressKey(m tea.Model, s string) tea.Cmd {
	_, cmd := m.Update(keyMsg(s))
	return cmd
}

// run runs cmd and what it leads to, as the program would, until it is done or quits.
func run(m tea.Model, cmd tea.Cmd) {
	for cmd != nil {
		msg := cmd()
		if _, quit := msg.(tea.QuitMsg); quit || msg == nil {
			return
		}
		_, cmd = m.Update(msg)
	}
}

// The new worktree form of another host's copy starts on the host's branch prefix, has the host create the
// branch typed, from the base typed, and opens it as the host's copy; what the host says when it cannot is
// the form's error.
func TestCopyNewForm(t *testing.T) {
	space := spaces.Space{Name: "homelab", Root: "/home/me/homelab", Config: &config.Config{Worktrees: config.Worktrees{Dir: ".worktrees", BranchPrefix: "me"}}}
	f := newCopyNewForm(config.Config{}, "white", space, spaces.BranchStart(*space.Config))
	f.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	var asked []string
	var opened []spaces.Worktree
	f.newWorktree = func(root, branch, base string) (spaces.Worktree, error) {
		asked = append(asked, root+" "+branch+" "+base)
		if branch == "me/taken" {
			return spaces.Worktree{}, errors.New("a branch named 'me/taken' already exists")
		}
		return spaces.Worktree{Path: root + "/.worktrees/" + branch[3:], Branch: branch, Session: "homelab@" + branch[3:]}, nil
	}
	f.open = func(wt spaces.Worktree) error { opened = append(opened, wt); return nil }
	if cmd := pressKey(f, "enter"); cmd != nil {
		t.Error("enter with the prefix alone ran something")
	}
	typeInto(f, "taken")
	run(f, pressKey(f, "enter"))
	if f.err == nil || f.err.Error() != "a branch named 'me/taken' already exists" {
		t.Errorf("error %v", f.err)
	}
	f.branch.fill("me/dns")
	pressKey(f, "tab")
	typeInto(f, "dev")
	run(f, pressKey(f, "enter"))
	if want := []string{"/home/me/homelab me/taken ", "/home/me/homelab me/dns dev"}; !reflect.DeepEqual(asked, want) {
		t.Errorf("asked %q, want %q", asked, want)
	}
	if want := []spaces.Worktree{{Path: "/home/me/homelab/.worktrees/dns", Branch: "me/dns", Session: "white:homelab@dns", Host: "white"}}; !reflect.DeepEqual(opened, want) {
		t.Errorf("opened %+v, want %+v", opened, want)
	}
}

// The remove form of another host's copy closes the copy's kitty session here, then has the host remove
// the worktree as chosen; one with changes is only force removed.
func TestCopyRemoveForm(t *testing.T) {
	wt := spaces.Worktree{Path: "/home/me/homelab/.worktrees/dns", Branch: "me/dns", Session: "white:homelab@dns", Host: "white"}
	var did []string
	form := func(changes int) *copyRemoveForm {
		f := &copyRemoveForm{th: newSidebar(0).th, host: "white", wt: wt, changes: changes,
			end: func(session string) error { did = append(did, "end "+session); return nil },
			remove: func(path string, opts spaces.RemoveOptions) error {
				did = append(did, "remove "+path+" "+map[bool]string{true: "force", false: "keep"}[opts.Force]+" "+map[bool]string{true: "branch", false: "no branch"}[opts.DeleteBranch])
				return nil
			}}
		f.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
		return f
	}
	f := form(0)
	run(f, pressKey(f, "y"))
	f = form(0)
	run(f, pressKey(f, "d"))
	f = form(3)
	if cmd := pressKey(f, "y"); cmd != nil {
		t.Error("y with changes ran something")
	}
	run(f, pressKey(f, "f"))
	want := []string{
		"end white:homelab@dns", "remove /home/me/homelab/.worktrees/dns keep no branch",
		"end white:homelab@dns", "remove /home/me/homelab/.worktrees/dns keep branch",
		"end white:homelab@dns", "remove /home/me/homelab/.worktrees/dns force no branch",
	}
	if !reflect.DeepEqual(did, want) {
		t.Errorf("did\n%q\nwant\n%q", did, want)
	}
}

// The forms say what they do, on which host.
func TestCopyFormScreens(t *testing.T) {
	space := spaces.Space{Name: "homelab", Root: "/home/me/homelab", Config: &config.Config{Worktrees: config.Worktrees{Dir: ".worktrees"}}}
	n := newCopyNewForm(config.Config{}, "white", space, "")
	n.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	typeInto(n, "dns")
	wt := spaces.Worktree{Path: "/home/me/homelab/.worktrees/dns", Branch: "dns", Session: "white:homelab@dns", Host: "white"}
	r := &copyRemoveForm{th: n.th, host: "white", wt: wt, changes: 2}
	r.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	got := [][]string{formScreen(n), formScreen(r)}
	want := [][]string{{
		"",
		"  New worktree in homelab on white",
		"",
		"  branch     dns",
		"  from       the main worktree's branch",
		"",
		"  Creates /home/me/homelab/.worktrees/dns on white, and opens it",
		"",
		"   Create enter    Cancel esc   tab next field",
		"",
	}, {
		"",
		"  Remove homelab@dns on white",
		"",
		"  Closes white:homelab@dns here, then removes /home/me/homelab/.worktrees/dns",
		"  on white, ending the sessions there tagged with it.",
		"  branch dns",
		"  2 files not committed, which only Force remove loses",
		"",
		"   Remove    Remove and delete branch    Force remove f    Cancel n",
		"",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("screens\n%q\nwant\n%q", got, want)
	}
}
