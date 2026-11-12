package spaces

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// node is a splits tree with its biases in whole percent, for comparing a tree with the one built back.
type node struct {
	Group      int
	Horizontal bool
	Bias       int
	One, Two   *node
}

// normal is s as a node, its groups renamed by rename; a pair with one side is that side.
func normal(s *kitty.Split, rename func(int) int) *node {
	switch {
	case s == nil:
		return nil
	case s.IsGroup():
		return &node{Group: rename(s.Group)}
	case s.Two == nil:
		return normal(s.One, rename)
	case s.One == nil:
		return normal(s.Two, rename)
	}
	return &node{Horizontal: s.Horizontal, Bias: int(math.Round(s.Bias * 100)), One: normal(s.One, rename), Two: normal(s.Two, rename)}
}

// splitKitty builds a splits tree from the launch actions building a layout asks for, as kitty's splits
// layout does: the tab's first window alone in a pair, then each split on window N's pair, filling the
// pair when it has one window, else replacing N with a pair of N and the new window, whose bias is
// 1-bias/100, the share of N.
func splitKitty(t *testing.T, calls [][]string) *kitty.Split {
	t.Helper()
	var root *kitty.Split
	id := 10
	var parent func(s *kitty.Split, win int) *kitty.Split
	parent = func(s *kitty.Split, win int) *kitty.Split {
		if s == nil || s.IsGroup() {
			return nil
		}
		for _, c := range []*kitty.Split{s.One, s.Two} {
			if c != nil && c.IsGroup() && c.Group == win {
				return s
			}
			if p := parent(c, win); p != nil {
				return p
			}
		}
		return nil
	}
	for _, call := range calls {
		if call[0] != "action" || call[1] == "goto_session" {
			continue
		}
		id++
		opts := map[string]string{}
		for _, a := range call {
			if k, v, ok := strings.Cut(strings.TrimPrefix(a, "--"), "="); ok {
				opts[k] = v
			}
		}
		if opts["type"] == "tab" {
			root = &kitty.Split{Horizontal: true, Bias: 0.5, One: &kitty.Split{Group: id}}
			continue
		}
		next, _ := strconv.Atoi(strings.TrimPrefix(opts["next-to"], "id:"))
		bias, _ := strconv.Atoi(opts["bias"])
		p := parent(root, next)
		if p == nil {
			t.Fatalf("no window %d to split in %v", next, calls)
		}
		h := opts["location"] == "vsplit"
		final := p
		if p.Two == nil {
			p.Horizontal, p.Two = h, &kitty.Split{Group: id}
		} else {
			final = &kitty.Split{Horizontal: h, One: &kitty.Split{Group: next}, Two: &kitty.Split{Group: id}}
			if p.One.IsGroup() && p.One.Group == next {
				p.One = final
			} else {
				p.Two = final
			}
		}
		final.Bias = 1 - float64(bias)/100
	}
	return root
}

func decodeSplit(t *testing.T, s string) *kitty.Split {
	t.Helper()
	var out kitty.Split
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		t.Fatal(err)
	}
	return &out
}

// A splits tree becomes panes top down: a pair's first window, then the first of its second side split
// off it, right when side by side, below when one above the other, at 1-bias, then each side the same.
// Building those panes makes kitty's tree again, the same pairs at the same biases.
func TestSplitPanes(t *testing.T) {
	for _, c := range []struct {
		name, tree string
		groups     []int
		panes      []config.Pane
	}{
		{"one window", `{"one": 5}`, []int{5}, []config.Pane{{}}},
		{
			// claude, gr right of it at 35%, a shell below claude at 30%: kitty's tree for the agent layout.
			"right then below", `{"bias": 0.65, "one": {"bias": 0.7, "horizontal": false, "one": 1, "two": 3}, "two": 2}`,
			[]int{1, 2, 3},
			[]config.Pane{{}, {Split: "right", Of: 1, Size: 35}, {Split: "below", Of: 1, Size: 30}},
		},
		{
			"left-right inside top-bottom", `{"horizontal": false, "bias": 0.6, "one": {"bias": 0.25, "one": 4, "two": 7}, "two": {"one": 9, "two": 2}}`,
			[]int{4, 9, 7, 2},
			[]config.Pane{{}, {Split: "below", Of: 1, Size: 40}, {Split: "right", Of: 1, Size: 75}, {Split: "right", Of: 2, Size: 50}},
		},
		{
			"three levels", `{"bias": 0.3, "one": 1, "two": {"horizontal": false, "bias": 0.55, "one": {"bias": 0.333, "one": 2, "two": 3}, "two": {"horizontal": false, "one": 4, "two": {"bias": 0.8, "one": 5, "two": 6}}}}`,
			[]int{1, 2, 4, 3, 5, 6},
			[]config.Pane{
				{}, {Split: "right", Of: 1, Size: 70}, {Split: "below", Of: 2, Size: 45}, {Split: "right", Of: 2, Size: 67},
				{Split: "below", Of: 3, Size: 50}, {Split: "right", Of: 5, Size: 20},
			},
		},
		{"extreme biases", `{"bias": 0.999, "one": 1, "two": {"horizontal": false, "bias": 0.001, "one": 2, "two": 3}}`,
			[]int{1, 2, 3}, []config.Pane{{}, {Split: "right", Of: 1, Size: 1}, {Split: "below", Of: 2, Size: 99}}},
	} {
		tree := decodeSplit(t, c.tree)
		// As capturing a tab does, a pair with one window is that window.
		groups, panes := splitPanes(prune(tree, func(int) bool { return true }))
		if !reflect.DeepEqual(groups, c.groups) || !reflect.DeepEqual(panes, c.panes) {
			t.Errorf("%s:\ngot  %v %+v\nwant %v %+v", c.name, groups, panes, c.groups, c.panes)
			continue
		}
		if c.name == "extreme biases" {
			// 1% and 99% are as far as a size goes, so the tree comes back at those.
			continue
		}
		// Built through the launch actions, window 11 is pane 1 and so on.
		cfg := config.Config{Launch: []string{"launch"}}
		lr := LayoutRun{Layout: config.Layout{Name: "saved", Tabs: []config.LayoutTab{{Panes: panes}}}}
		k := &fakeLayoutKitty{}
		if _, err := buildLayout(cfg, kitty.State{}, Space{Name: "gr", Root: "/src/gr"}, Worktree{Path: "/src/gr", Session: "gr"}, lr, k); err != nil {
			t.Fatal(err)
		}
		built := normal(splitKitty(t, k.calls), func(id int) int { return groups[id-11] })
		if want := normal(tree, func(g int) int { return g }); !reflect.DeepEqual(built, want) {
			t.Errorf("%s: built back as\n%s\nwant\n%s", c.name, show(built), show(want))
		}
	}
}

func show(n *node) string {
	if n == nil {
		return "nil"
	}
	if n.One == nil {
		return fmt.Sprint(n.Group)
	}
	return fmt.Sprintf("(%v %d %s %s)", n.Horizontal, n.Bias, show(n.One), show(n.Two))
}

// The tab kitty lists for the agent layout in gr@fix: claude in cm session c1 at window 1 with a form over
// it, gr in cm's c2 right of it, which cm says runs gr, and below claude a shell in c3; the sidebar docked
// at its left. The title is the branch's, set when it opened, and gr's pane has focus.
const agentTab = `{
  "id": 1, "is_active": true, "title": "pr/me/fix: {x}", "title_overridden": true, "layout": "splits",
  "layout_state": {
    "pairs": {"bias": 0.65, "one": {"bias": 0.7, "horizontal": false, "one": 1, "two": 3}, "two": 2},
    "all_windows": {"active_group_idx": 2, "window_groups": [
      {"id": 1, "window_ids": [1, 5]}, {"id": 3, "window_ids": [3]}, {"id": 2, "window_ids": [2]}
    ]}
  },
  "windows": [
    {"id": 1, "session_name": "gr@fix", "cwd": "/home", "cmdline": ["/usr/local/bin/cm", "attach", "c1"],
     "last_reported_cmdline": "claude --resume abc", "at_prompt": false},
    {"id": 2, "session_name": "gr@fix", "cwd": "/home", "cmdline": ["cm", "attach", "--if-running=attach", "c2"],
     "last_reported_cmdline": "", "at_prompt": false},
    {"id": 3, "session_name": "gr@fix", "cwd": "/src/gr/.worktrees/fix", "cmdline": ["cm", "attach", "c3"],
     "last_reported_cmdline": "ls", "at_prompt": true},
    {"id": 4, "session_name": "gr@fix", "cmdline": ["kspaces", "sidebar"], "user_vars": {"kspaces": "1"}},
    {"id": 5, "session_name": "gr@fix", "cmdline": ["kspaces", "layout", "save"], "user_vars": {"kspaces_form": "1"}}
  ]
}`

// A second tab in tall: make watch under --hold, a shell at its prompt that ran a command, focused, and
// one in /tmp in no shell integration; its title kitty's own.
const watchTab = `{
  "id": 2, "title": "make", "title_overridden": false, "layout": "tall",
  "layout_state": {
    "all_windows": {"active_group_idx": 1, "window_groups": [
      {"id": 6, "window_ids": [6]}, {"id": 7, "window_ids": [7]}, {"id": 8, "window_ids": [8]}
    ]}
  },
  "windows": [
    {"id": 6, "session_name": "gr@fix", "cwd": "/src/gr/.worktrees/fix/web", "cmdline": ["make", "watch"],
     "foreground_processes": [
       {"cmdline": ["make", "watch"]},
       {"cmdline": ["/Applications/kitty.app/Contents/MacOS/kitten", "run-shell", "--shell=/bin/zsh", "make", "watch"]}
     ]},
    {"id": 7, "session_name": "gr@fix", "cwd": "/src/gr/.worktrees/fix", "cmdline": ["-zsh"],
     "foreground_processes": [{"cmdline": ["-zsh"]}], "last_reported_cmdline": "git status", "at_prompt": true},
    {"id": 8, "session_name": "gr@fix", "cwd": "/tmp", "cmdline": ["/bin/sh"], "foreground_processes": [{"cmdline": ["/bin/sh"]}]}
  ]
}`

func decodeTab(t *testing.T, s string) kitty.FullTab {
	t.Helper()
	var tab kitty.FullTab
	if err := json.Unmarshal([]byte(s), &tab); err != nil {
		t.Fatal(err)
	}
	return tab
}

// A tab is captured as panes that build its arrangement back, leaving out kspaces' docks and forms, each
// with what runs in it and its cwd as a template of the worktree's; the title, when set, with the branch
// as {branch}; the focused pane marked, unless it is the first. Another layout is its name and the
// windows in order.
func TestCaptureTab(t *testing.T) {
	wt := Worktree{Path: "/src/gr/.worktrees/fix", Branch: "pr/me/fix", Session: "gr@fix"}
	cm := map[string]cmSession{
		"c1": {Name: "c1", Command: "claude", Cwd: "/src/gr/.worktrees/fix"},
		"c2": {Name: "c2", Command: "gr --watch", Cwd: "/src/gr/.worktrees/fix"},
		"c3": {Name: "c3", Cwd: "/src/gr/.worktrees/fix/docs"},
	}
	got := []CapturedTab{captureTab(decodeTab(t, agentTab), wt, cm), captureTab(decodeTab(t, watchTab), wt, cm)}
	want := []CapturedTab{
		{
			Tab: config.LayoutTab{Title: "{branch}: {{x}}", Panes: []config.Pane{
				{},
				{Split: "right", Of: 1, Size: 35, Focus: true},
				{Split: "below", Of: 1, Size: 30, Cwd: "{worktree}/docs"},
			}},
			Programs: []string{"claude --resume abc", "gr --watch", ""},
		},
		{
			Tab:      config.LayoutTab{Layout: "tall", Panes: []config.Pane{{Cwd: "{worktree}/web"}, {Focus: true}, {Cwd: "/tmp"}}},
			Programs: []string{"make watch", "", ""},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// A maximized window's splits are captured at the biases they had before, which kitty keeps; a window
// group of only kspaces' own, such as a form in a split, is no pane, its sibling taking its place.
func TestCaptureTabMaximizedAndPruned(t *testing.T) {
	tab := decodeTab(t, `{
  "layout": "splits",
  "layout_state": {
    "pairs": {"bias": 1.0, "one": 1, "two": {"horizontal": false, "bias": 0.0, "one": 2, "two": 3}},
    "maximized": [{"window_id": 1, "is_horizontal": true, "saved_biases": [{"path": [], "bias": 0.6}, {"path": ["two"], "bias": 0.25}]}],
    "all_windows": {"active_group_idx": 0, "window_groups": [{"id": 1, "window_ids": [1]}, {"id": 2, "window_ids": [2]}, {"id": 3, "window_ids": [3]}]}
  },
  "windows": [
    {"id": 1, "session_name": "gr", "cwd": "/src/gr"},
    {"id": 2, "session_name": "gr", "cwd": "/src/gr"},
    {"id": 3, "session_name": "gr", "user_vars": {"kspaces_form": "1"}}
  ]
}`)
	got := captureTab(tab, Worktree{Path: "/src/gr", Branch: "main", Session: "gr"}, nil)
	want := CapturedTab{Tab: config.LayoutTab{Panes: []config.Pane{{}, {Split: "right", Of: 1, Size: 40}}}, Programs: []string{"", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// What runs in a window: the command its shell reports running; for cm attach, else what cm says its
// session runs, and cm's cwd; else a program in its foreground that is not a shell; else the shell.
func TestDetect(t *testing.T) {
	cm := map[string]cmSession{
		"busy": {Name: "busy", Command: "gr --watch", Cwd: "/src/gr/web"},
		"idle": {Name: "idle", Cwd: "/src/gr/docs"},
	}
	type fg = []struct {
		Cmdline []string `json:"cmdline"`
		Cwd     string   `json:"cwd"`
	}
	for _, c := range []struct {
		name      string
		w         kitty.FullWindow
		prog, cwd string
	}{
		{"cm attach, running a command cm knows", kitty.FullWindow{Cwd: "/x", Cmdline: []string{"cm", "attach", "busy"}}, "gr --watch", "/src/gr/web"},
		{"cm attach, at the prompt", kitty.FullWindow{Cwd: "/x", Cmdline: []string{"cm", "attach", "idle"}, AtPrompt: true}, "", "/src/gr/docs"},
		{"cm attach, the shell reporting", kitty.FullWindow{Cwd: "/x", Cmdline: []string{"cm", "attach", "idle"}, LastReportedCmdline: "claude --resume abc"},
			"claude --resume abc", "/src/gr/docs"},
		{"cm attach, a session cm does not list", kitty.FullWindow{Cwd: "/x", Cmdline: []string{"cm", "attach", "gone"}}, "", "/x"},
		{"a plain shell", kitty.FullWindow{Cwd: "/x", Cmdline: []string{"-zsh"}, ForegroundProcesses: fg{{Cmdline: []string{"-zsh"}}}}, "", "/x"},
		{"at the prompt after a command", kitty.FullWindow{Cwd: "/x", Cmdline: []string{"/bin/zsh"}, LastReportedCmdline: "make", AtPrompt: true,
			ForegroundProcesses: fg{{Cmdline: []string{"/bin/zsh"}}}}, "", "/x"},
		{"running a command, reported", kitty.FullWindow{Cwd: "/x", Cmdline: []string{"/bin/zsh"}, LastReportedCmdline: "make watch",
			ForegroundProcesses: fg{{Cmdline: []string{"make", "watch"}}}}, "make watch", "/x"},
		{"running a program, no shell integration", kitty.FullWindow{Cwd: "/x", Cmdline: []string{"/bin/sh"},
			ForegroundProcesses: fg{{Cmdline: []string{"/bin/sh"}}, {Cmdline: []string{"tail", "-f", "a log"}}}}, "tail -f 'a log'", "/x"},
		{"a held command", kitty.FullWindow{Cwd: "/x", Cmdline: []string{"htop"},
			ForegroundProcesses: fg{{Cmdline: []string{"kitten", "run-shell", "htop"}}, {Cmdline: []string{"htop"}}}}, "htop", "/x"},
	} {
		prog, cwd := detect(c.w, cm)
		if prog != c.prog || cwd != c.cwd {
			t.Errorf("%s: got %q in %q, want %q in %q", c.name, prog, cwd, c.prog, c.cwd)
		}
	}
}

// The session's tabs are those with a window of its own; this tab is the one the window asking is in,
// even as kspaces' own, else the one focused most recently.
func TestSessionTabs(t *testing.T) {
	tabs := []kitty.FullTab{
		{ID: 1, Windows: []kitty.FullWindow{{ID: 1, SessionName: "gr", LastFocusedAt: 5}}},
		{ID: 2, Windows: []kitty.FullWindow{{ID: 2, SessionName: "kitty", LastFocusedAt: 9}}},
		{ID: 3, Windows: []kitty.FullWindow{{ID: 3, SessionName: "gr", LastFocusedAt: 7}, {ID: 4, SessionName: "gr", UserVars: map[string]string{"kspaces": "1"}}}},
		{ID: 4, Windows: []kitty.FullWindow{{ID: 5, SessionName: "gr", UserVars: map[string]string{"kspaces": "1"}}}},
	}
	type result struct {
		IDs  []int
		This int
	}
	var got []result
	for _, self := range []int{1, 4, 2, 0} {
		out, this := sessionTabs(tabs, "gr", self)
		r := result{This: this}
		for _, tab := range out {
			r.IDs = append(r.IDs, tab.ID)
		}
		got = append(got, r)
	}
	want := []result{{[]int{1, 3}, 0}, {[]int{1, 3}, 1}, {[]int{1, 3}, 1}, {[]int{1, 3}, 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

// cwd is the worktree by default, {worktree}/SUB in it, else the path; braces are doubled. A worktree
// reached through a symlink is the same worktree, as kitty gives the cwd with symlinks resolved.
func TestCwdTemplate(t *testing.T) {
	var got []string
	for _, cwd := range []string{"", "/src/gr", "/src/gr/docs/a", "/src/grx", "/tmp/{x}", "/real/gr/docs"} {
		got = append(got, cwdTemplate(cwd, "/src/gr", "/real/gr"))
	}
	if want := []string{"", "", "{worktree}/docs/a", "/src/grx", "/tmp/{{x}}", "{worktree}/docs"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
	real := t.TempDir()
	if err := os.Mkdir(filepath.Join(real, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	real, _ = filepath.EvalSymlinks(real)
	link := filepath.Join(t.TempDir(), "gr")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	tab := decodeTab(t, fmt.Sprintf(`{"layout": "stack", "layout_state": {"all_windows": {"window_groups": [{"id": 1, "window_ids": [1]}, {"id": 2, "window_ids": [2]}]}},
  "windows": [{"id": 1, "session_name": "gr", "cwd": %q}, {"id": 2, "session_name": "gr", "cwd": %q}]}`, real, filepath.Join(real, "docs")))
	c := captureTab(tab, Worktree{Path: link, Session: "gr"}, nil)
	if want := []config.Pane{{}, {Cwd: "{worktree}/docs"}}; !reflect.DeepEqual(c.Tab.Panes, want) {
		t.Errorf("through a symlink: got %+v, want %+v", c.Tab.Panes, want)
	}
}

// A command line splits as a shell would, and words join into a line that splits back into them.
func TestWords(t *testing.T) {
	type result struct {
		Words []string
		Err   string
	}
	var got []result
	for _, line := range []string{`claude --resume abc`, `  a  'b c' "d \"e\" \x" f\ g''h `, `a 'b`, `a "b`, ``} {
		w, err := SplitWords(line)
		got = append(got, result{w, errString(err)})
	}
	want := []result{
		{[]string{"claude", "--resume", "abc"}, "<nil>"},
		{[]string{"a", "b c", `d "e" \x`, "f gh"}, "<nil>"},
		{nil, "a 'b has a ' that does not end"},
		{nil, `a "b has a " that does not end`},
		{nil, "<nil>"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
	words := []string{"make", "watch", "a b", "it's", "", "$HOME", "{x}"}
	line := JoinWords(words)
	back, err := SplitWords(line)
	if line != `make watch 'a b' 'it'\''s' '' '$HOME' '{x}'` || err != nil || !reflect.DeepEqual(back, words) {
		t.Errorf("joined as %s, split back as %q, %v", line, back, err)
	}
}
