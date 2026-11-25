package spaces

import (
	"path/filepath"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/kittyglue"
	"github.com/chancez/kspaces/internal/logs"
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

// popup is where cfg opens a popup of size for the window from, the default for a size it has none of. A
// size by the screen's is of from's screen, and the default where that is not known.
func popup(cfg config.Config, size panelSize, from int) kitty.Popup {
	if cfg.Sidebar.Popup != config.PopupPanel {
		return kitty.Popup{}
	}
	s := size.of(cfg.Sidebar.PopupSize)
	if s == (config.PopupSize{}) {
		s = size.of(config.DefaultPopupSizes)
	}
	columns, lines := s.Columns, s.Lines
	if s.Relative() {
		sc, sl, err := screenCells(from)
		if err == nil {
			columns, lines = s.Cells(sc, sl)
		} else {
			logs.Error("sizing a popup by the screen", err)
			d := size.of(config.DefaultPopupSizes)
			columns, lines = d.Columns, d.Lines
		}
	}
	return kitty.Popup{Panel: true, Columns: columns, Lines: lines}
}

// screenCells is how many of window's cells fit its screen, as the installed kspaces_screen.py says; a
// variable for tests.
var screenCells = func(window int) (int, int, error) {
	dir, err := kittyglue.DefaultDir()
	if err != nil {
		return 0, 0, err
	}
	return kitty.ScreenCells(filepath.Join(dir, kittyglue.ScreenScript), window)
}

// launchForm opens the form args run, such as `new ROOT`, in a popup of size for the window of st's tab.
func launchForm(cfg config.Config, st kitty.State, size panelSize, args ...string) error {
	return kitty.LaunchForm(popup(cfg, size, st.Home), st, args...)
}

// LaunchPicker opens the picker in a popup for the window from, as the kitty mapping that runs
// `kspaces pick --from` asks for, with pick's args, such as --scope.
func LaunchPicker(cfg config.Config, from int, args ...string) error {
	return kitty.LaunchPopup(popup(cfg, pickerPanel, from), from, "kspaces_picker", append([]string{"pick"}, args...)...)
}
