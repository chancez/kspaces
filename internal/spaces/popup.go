package spaces

import (
	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// panelSize is which of the config's panel sizes a popup has, by what it shows.
type panelSize int

const (
	// A form of a few fields or a question.
	formPanel panelSize = iota
	// A form that lists what it acts on.
	listPanel
	// The picker.
	pickerPanel
)

// of is the size of sizes this one is.
func (size panelSize) of(sizes config.PopupSizes) config.PopupSize {
	switch size {
	case listPanel:
		return sizes.List
	case pickerPanel:
		return sizes.Picker
	}
	return sizes.Form
}

// popup is where cfg opens a popup of size, the default for a size it has none of.
func popup(cfg config.Config, size panelSize) kitty.Popup {
	if cfg.Sidebar.Popup != config.PopupPanel {
		return kitty.Popup{}
	}
	s := size.of(cfg.Sidebar.PopupSize)
	if s == (config.PopupSize{}) {
		s = size.of(config.DefaultPopupSizes)
	}
	return kitty.Popup{Panel: true, Columns: s.Columns, Lines: s.Lines}
}

// launchForm opens the form args run, such as `new ROOT`, in a popup of size for the window of st's tab.
func launchForm(cfg config.Config, st kitty.State, size panelSize, args ...string) error {
	return kitty.LaunchForm(popup(cfg, size), st, args...)
}

// LaunchPicker opens the picker in a popup for the window from, as the kitty mapping that runs
// `kspaces pick --from` asks for, with pick's args, such as --scope.
func LaunchPicker(cfg config.Config, from int, args ...string) error {
	return kitty.LaunchPopup(popup(cfg, pickerPanel), from, "kspaces_picker", append([]string{"pick"}, args...)...)
}
