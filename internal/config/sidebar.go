package config

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/chancez/kspaces/internal/logs"
)

// sidebarWidthKey is the setting a dragged sidebar's width is saved as.
var sidebarWidthKey = []string{"sidebar", "width"}

// SaveSidebarWidth sets the sidebar's width to width columns, for the docks of new sidebars, see
// setSidebarWidth and EditFile, in the file that sets it, else the one with a [sidebar] table, else
// config.toml, checked merged with the other config files. A missing config.toml is created; the width
// it already is writes nothing.
func SaveSidebarWidth(width int) (err error) {
	defer func(start time.Time) { logs.Action("save sidebar width", start, err, "width", width) }(time.Now())
	files, cfg, err := readMerged()
	if err != nil {
		return err
	}
	if cfg.Sidebar.Width == width {
		return nil
	}
	i, err := sidebarFile(files)
	if err != nil {
		return err
	}
	return EditFile(files.paths[i], func(text string) (string, error) {
		out, err := setSidebarWidth(text, width)
		if err != nil {
			return "", err
		}
		after, _, err := files.with(i, out)
		if err != nil {
			return "", fmt.Errorf("setting sidebar.width to %d would break the config, so it was left as is: %w", width, err)
		}
		if after.Sidebar.Width != width {
			return "", fmt.Errorf("setting sidebar.width to %d would make it %d, so the config was left as is", width, after.Sidebar.Width)
		}
		return out, nil
	})
}

// sidebarFile is the index in files of the file the sidebar's width goes in: the one that sets it, which
// only one can, else the first with a [sidebar] table, else config.toml.
func sidebarFile(files configFiles) (int, error) {
	table := -1
	for i, text := range files.texts {
		p, err := decode(text)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", files.paths[i], err)
		}
		if p.md.IsDefined(sidebarWidthKey...) {
			return i, nil
		}
		if table >= 0 {
			continue
		}
		sc := scanner{s: text}
		headers, err := sc.tables()
		if err != nil {
			return 0, fmt.Errorf("%s: %w", files.paths[i], err)
		}
		if slices.ContainsFunc(headers, func(h tableHeader) bool { return !h.array && slices.Equal(h.key, sidebarWidthKey[:1]) }) {
			table = i
		}
	}
	return max(table, 0), nil
}

// setSidebarWidth is text with the sidebar's width set to width: its value replaced where it is set, in
// [sidebar] or as a dotted key, else a width key added right under the [sidebar] header, else a [sidebar]
// table added at the end.
func setSidebarWidth(text string, width int) (string, error) {
	sc := scanner{s: text}
	start, end, header, err := sc.setting(sidebarWidthKey)
	if err != nil {
		return "", err
	}
	v := strconv.Itoa(width)
	switch {
	case start >= 0:
		return text[:start] + v + text[end:], nil
	case header >= 0:
		at := strings.IndexByte(text[header:], '\n')
		if at < 0 {
			return text + "\nwidth = " + v + "\n", nil
		}
		at += header + 1
		return text[:at] + "width = " + v + "\n" + text[at:], nil
	}
	return appendBlock(text, "[sidebar]\nwidth = "+v+"\n"), nil
}

// setting is where the value of the key at path starts and ends, set in its table or as a dotted key,
// both -1 when the text does not set it, and where the [table] header of the table path is in starts, -1
// without one.
func (sc *scanner) setting(path []string) (start, end, header int, err error) {
	text := sc.s
	table, parent, arrayTable := []string(nil), path[:len(path)-1], false
	header = -1
	for {
		i := sc.skipBlank(sc.i)
		if i >= len(text) {
			return -1, -1, header, nil
		}
		if text[i] == '[' {
			h, j, err := sc.header(i)
			if err != nil {
				return 0, 0, 0, err
			}
			// A key in an array of tables is never path's.
			table, arrayTable = h.key, h.array
			if !h.array && slices.Equal(h.key, parent) {
				header = h.start
			}
			sc.i = j
			continue
		}
		key, j, err := sc.key(i)
		if err != nil {
			return 0, 0, 0, err
		}
		j = sc.skipSpace(j)
		if j >= len(text) || text[j] != '=' {
			return 0, 0, 0, fmt.Errorf("line %d: expected = after the key", sc.line(j))
		}
		v := sc.skipSpace(j + 1)
		if sc.i, err = sc.value(v); err != nil {
			return 0, 0, 0, err
		}
		if !arrayTable && slices.Equal(append(slices.Clone(table), key...), path) {
			// A bare value's end takes the spaces before a comment after it.
			return v, v + len(strings.TrimRight(text[v:sc.i], " \t")), header, nil
		}
	}
}
