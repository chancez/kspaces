package spaces

import (
	"strconv"
	"strings"
)

// The height of the sidebar's STATUS list in rows, as dragging the rule above it left it. Shared, as the
// folds are, so every sidebar opens at it after a restart.
const splitFile = "split"

// LoadSplit reads the STATUS list's height. A missing or unreadable file is 0, for the default.
func LoadSplit() int {
	n, err := strconv.Atoi(strings.TrimSpace(string(readState(splitFile))))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// SaveSplit writes the STATUS list's height.
func SaveSplit(rows int) error {
	return writeState(splitFile, []byte(strconv.Itoa(rows)+"\n"))
}
