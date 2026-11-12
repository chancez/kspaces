package spaces

import (
	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// panelSize is a panel's size in cells. A panel does not grow with what it shows, so each is sized for
// what its popup shows at its longest.
type panelSize struct{ columns, lines int }

var (
	// A form of a few fields or a question.
	formPanel = panelSize{90, 24}
	// A form that lists what it acts on: the tabs a layout saves, the worktrees a clean up removes, what a
	// space's own config adds.
	listPanel = panelSize{100, 32}
	// The picker, wide enough for its preview beside the list.
	pickerPanel = panelSize{120, 32}
)

// popup is where cfg opens a popup of size.
func popup(cfg config.Config, size panelSize) kitty.Popup {
	if cfg.Sidebar.Popup != config.PopupPanel {
		return kitty.Popup{}
	}
	return kitty.Popup{Panel: true, Columns: size.columns, Lines: size.lines}
}

// launchForm opens the form args run, such as `new ROOT`, in a popup of size for the window of st's tab.
func launchForm(cfg config.Config, st kitty.State, size panelSize, args ...string) error {
	return kitty.LaunchForm(popup(cfg, size), st, args...)
}

// LaunchPicker opens the picker in a popup for the window from, as the kitty mapping that runs
// `kspaces pick --from` asks for.
func LaunchPicker(cfg config.Config, from int) error {
	return kitty.LaunchPopup(popup(cfg, pickerPanel), from, "kspaces_picker", "pick")
}
