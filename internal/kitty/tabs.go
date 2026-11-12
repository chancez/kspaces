package kitty

import (
	"encoding/json"
	"fmt"
)

// The tabs of `kitten @ ls` in full, for saving a session's tabs as a layout: how each is laid out and
// what runs in its windows. The sidebar's poll reads only the part State needs, so it decodes no more.

// FullTab is a tab in `kitten @ ls`, with its layout.
type FullTab struct {
	ID       int    `json:"id"`
	IsActive bool   `json:"is_active"`
	Title    string `json:"title"`
	// Whether the title was set, by --tab-title or set_tab_title, rather than taken from a window.
	TitleOverridden bool         `json:"title_overridden"`
	Layout          string       `json:"layout"`
	LayoutState     LayoutState  `json:"layout_state"`
	Windows         []FullWindow `json:"windows"`
}

// LayoutState is how a tab's windows are laid out: its window groups in order, a window and the overlays
// over it each, and in the splits layout the tree of splits between them.
type LayoutState struct {
	// The splits layout's tree, nil in any other layout.
	Pairs *Split `json:"pairs"`
	// The biases a maximized window's splits had before, which unmaximizing it puts back.
	Maximized []struct {
		SavedBiases []struct {
			// From the tree's root, "one" or "two" at each pair.
			Path []string `json:"path"`
			Bias float64  `json:"bias"`
		} `json:"saved_biases"`
	} `json:"maximized"`
	AllWindows struct {
		ActiveGroupIdx int `json:"active_group_idx"`
		WindowGroups   []struct {
			ID        int   `json:"id"`
			WindowIDs []int `json:"window_ids"`
		} `json:"window_groups"`
	} `json:"all_windows"`
}

// Split is a node of the splits layout's tree: a window group, by its id, or a pair of nodes side by side
// when Horizontal, else One above Two, One taking Bias of the room.
type Split struct {
	Group      int
	Horizontal bool
	Bias       float64
	// Nil for a group; a pair with one window has no Two.
	One, Two *Split
}

// IsGroup says whether s is a window group rather than a pair.
func (s *Split) IsGroup() bool { return s.One == nil && s.Two == nil }

// UnmarshalJSON reads a group's id or a pair, which leaves out horizontal when true and bias when 0.5.
func (s *Split) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] != '{' {
		return json.Unmarshal(data, &s.Group)
	}
	pair := struct {
		Horizontal *bool    `json:"horizontal"`
		Bias       *float64 `json:"bias"`
		One        *Split   `json:"one"`
		Two        *Split   `json:"two"`
	}{}
	if err := json.Unmarshal(data, &pair); err != nil {
		return err
	}
	*s = Split{Horizontal: pair.Horizontal == nil || *pair.Horizontal, Bias: 0.5, One: pair.One, Two: pair.Two}
	if pair.Bias != nil {
		s.Bias = *pair.Bias
	}
	if s.IsGroup() {
		return fmt.Errorf("a split with neither one nor two")
	}
	return nil
}

// FullWindow is a window in `kitten @ ls`, with what runs in it.
type FullWindow struct {
	ID            int               `json:"id"`
	IsActive      bool              `json:"is_active"`
	SessionName   string            `json:"session_name"`
	Cwd           string            `json:"cwd"`
	LastFocusedAt float64           `json:"last_focused_at"`
	UserVars      map[string]string `json:"user_vars"`
	// The window's program, as it was started.
	Cmdline []string `json:"cmdline"`
	// The processes in the foreground of its terminal, such as a command its shell runs.
	ForegroundProcesses []struct {
		Cmdline []string `json:"cmdline"`
		Cwd     string   `json:"cwd"`
	} `json:"foreground_processes"`
	// With shell integration, the last command line the shell ran, and whether it is at its prompt again.
	LastReportedCmdline string `json:"last_reported_cmdline"`
	AtPrompt            bool   `json:"at_prompt"`
}

// Own says whether w is one of kspaces' own windows, as ownWindow does.
func (w FullWindow) Own() bool { return ownWindow(Window{UserVars: w.UserVars}) }

// ListTabs is every tab of `kitten @ ls`, in full.
func ListTabs() ([]FullTab, error) {
	out, err := kitten("ls")
	if err != nil {
		return nil, err
	}
	var data []struct {
		Tabs []FullTab `json:"tabs"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, fmt.Errorf("parsing kitten @ ls: %w", err)
	}
	var tabs []FullTab
	for _, osw := range data {
		tabs = append(tabs, osw.Tabs...)
	}
	return tabs, nil
}
