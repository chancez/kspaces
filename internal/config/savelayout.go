package config

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Saving a layout edits config.toml as text, as AddSpace does: the layout is a [[layouts]] block of its
// own, at the end or in place of the one it replaces, and the rest of the file is kept byte for byte.

// AddLayout is the config text with l, a layout as Parse gives it, written as a [[layouts]] block at the
// end; or, when replace says to, in place of the layout called the same, which is otherwise an error. The
// result is parsed back and must have l as it is, where it goes, and nothing else changed, so a mistake in
// the edit is an error rather than a broken config.
func AddLayout(text string, l Layout, replace bool) (string, error) {
	return addLayout(text, l, replace, func(text string) (Config, []string, error) {
		cfg, err := Parse(text)
		return cfg, layoutNames(cfg.Layouts), err
	})
}

// AddSpaceLayout is AddLayout for the text of a space's file, the space called space with global as its
// global config: replace is of a layout the file has, and one only the global config has is added to the
// file, to take its place in the space. It is checked as merged with global.
func AddSpaceLayout(text string, global Config, space string, l Layout, replace bool) (string, error) {
	return addLayout(text, l, replace, func(text string) (Config, []string, error) {
		f, err := ParseSpaceFile(text)
		if err != nil {
			return Config{}, nil, err
		}
		cfg, err := global.WithSpace(space, f)
		return cfg, layoutNames(f.Layouts), err
	})
}

// addLayout is AddLayout with parse reading a text as the config it makes and the names of the layouts the
// text itself has, in order.
func addLayout(text string, l Layout, replace bool, parse func(string) (Config, []string, error)) (string, error) {
	before, own, err := parse(text)
	if err != nil {
		return "", fmt.Errorf("the config has an error to fix first: %w", err)
	}
	block := layoutBlock(l)
	want := before
	want.Layouts = slices.Clone(before.Layouts)
	var out string
	switch at := slices.Index(own, l.Name); {
	case at < 0:
		out = appendBlock(text, block)
		// A layout of the same name from elsewhere, the global config's, is taken the place of.
		if i := slices.IndexFunc(want.Layouts, func(x Layout) bool { return x.Name == l.Name }); i >= 0 {
			want.Layouts[i] = l
		} else {
			want.Layouts = append(want.Layouts, l)
		}
	case !replace:
		return "", fmt.Errorf("a layout called %s exists", l.Name)
	default:
		if out, err = replaceLayout(text, at, len(own), block); err != nil {
			return "", err
		}
		want.Layouts[slices.IndexFunc(want.Layouts, func(x Layout) bool { return x.Name == l.Name })] = l
	}
	after, _, err := parse(out)
	if err != nil {
		return "", fmt.Errorf("saving layout %s would break the config, so it was left as is: %w", l.Name, err)
	}
	// The inputs are what Parse works out from the commands, which are as they were.
	for i := range want.Layouts {
		if i < len(after.Layouts) && want.Layouts[i].Name == l.Name {
			want.Layouts[i].Inputs = after.Layouts[i].Inputs
		}
	}
	if !reflect.DeepEqual(after, want) {
		return "", fmt.Errorf("saving layout %s would change more of the config than it, so the config was left as is", l.Name)
	}
	return out, nil
}

func layoutNames(ls []Layout) []string {
	names := make([]string, len(ls))
	for i, l := range ls {
		names[i] = l.Name
	}
	return names
}

// SaveLayout adds l to the config file Load reads, or replaces the one called the same when replace says
// to, see AddLayout and editFile.
func SaveLayout(l Layout, replace bool) error {
	return editFile(func(text string) (string, error) { return AddLayout(text, l, replace) })
}

// layoutBlock is l as a [[layouts]] table, each tab a [[layouts.tabs]] with its panes inline, a line each.
// What a pane leaves at its default is left out: of for the pane before, size 50, the worktree's cwd.
func layoutBlock(l Layout) string {
	var b strings.Builder
	b.WriteString("[[layouts]]\nname = " + tomlString(l.Name) + "\n")
	for _, tab := range l.Tabs {
		b.WriteString("[[layouts.tabs]]\n")
		if tab.Title != "" {
			b.WriteString("title = " + tomlString(tab.Title) + "\n")
		}
		if tab.Layout != "" {
			b.WriteString("layout = " + tomlString(tab.Layout) + "\n")
		}
		b.WriteString("panes = [\n")
		for i, p := range tab.Panes {
			b.WriteString("  " + paneTable(p, i) + ",\n")
		}
		b.WriteString("]\n")
	}
	return b.String()
}

// paneTable is p, pane i from 0 of its tab, as an inline table.
func paneTable(p Pane, i int) string {
	var kv []string
	switch {
	case p.Command != "":
		kv = append(kv, "command = "+tomlString(p.Command))
	case p.Run != nil:
		words := make([]string, len(p.Run))
		for j, w := range p.Run {
			words[j] = tomlString(w)
		}
		kv = append(kv, "run = ["+strings.Join(words, ", ")+"]")
	}
	if p.Split != "" {
		kv = append(kv, "split = "+tomlString(p.Split))
	}
	if p.Of != 0 && p.Of != i {
		kv = append(kv, "of = "+strconv.Itoa(p.Of))
	}
	if p.Size != 0 && p.Size != 50 {
		kv = append(kv, "size = "+strconv.Itoa(p.Size))
	}
	if p.Cwd != "" {
		kv = append(kv, "cwd = "+tomlString(p.Cwd))
	}
	if p.Focus {
		kv = append(kv, "focus = true")
	}
	if len(kv) == 0 {
		return "{}"
	}
	return "{ " + strings.Join(kv, ", ") + " }"
}

// appendBlock is text with block after it, a blank line between.
func appendBlock(text, block string) string {
	if strings.TrimSpace(text) == "" {
		return text + block
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if !strings.HasSuffix(text, "\n\n") {
		text += "\n"
	}
	return text + block
}

// replaceLayout is text with the layout at index at of the n it has replaced by block: from its
// [[layouts]] line to the next table that is not one of its own, leaving the blank lines before that and
// the comments right above it, which belong to it.
func replaceLayout(text string, at, n int, block string) (string, error) {
	sc := scanner{s: text}
	headers, err := sc.tables()
	if err != nil {
		return "", err
	}
	isLayout := func(h tableHeader) bool { return h.array && slices.Equal(h.key, []string{"layouts"}) }
	if len(slices.DeleteFunc(slices.Clone(headers), func(h tableHeader) bool { return !isLayout(h) })) != n {
		return "", fmt.Errorf("the config's layouts are not all [[layouts]] tables, so one cannot be replaced in place")
	}
	k, start, end := -1, -1, len(text)
	for _, h := range headers {
		own := len(h.key) > 1 && h.key[0] == "layouts"
		switch {
		case isLayout(h):
			k++
			if k == at {
				start = lineStart(text, h.start)
				continue
			}
		case own:
			continue
		}
		if start >= 0 {
			end = commentsAbove(text, lineStart(text, h.start))
			break
		}
	}
	for end > start {
		prev := lineStart(text, end-1)
		if strings.TrimSpace(text[prev:end]) != "" {
			break
		}
		end = prev
	}
	return text[:start] + block + text[end:], nil
}

// tableHeader is a table's header: where its [ is, its key, and whether it is an array of tables.
type tableHeader struct {
	start int
	key   []string
	array bool
}

// tables are the table headers of the text, in order, stepping over the values of the keys between them.
func (sc *scanner) tables() ([]tableHeader, error) {
	text := sc.s
	var out []tableHeader
	for {
		i := sc.skipBlank(sc.i)
		if i >= len(text) {
			return out, nil
		}
		if text[i] == '[' {
			array := strings.HasPrefix(text[i:], "[[")
			close := "]"
			j := i + 1
			if array {
				close, j = "]]", i+2
			}
			key, j, err := sc.key(j)
			if err != nil {
				return nil, err
			}
			if !strings.HasPrefix(text[j:], close) {
				return nil, fmt.Errorf("line %d: expected %s after the table's name", sc.line(j), close)
			}
			out = append(out, tableHeader{start: i, key: key, array: array})
			sc.i = j + len(close)
			continue
		}
		_, j, err := sc.key(i)
		if err != nil {
			return nil, err
		}
		j = sc.skipSpace(j)
		if j >= len(text) || text[j] != '=' {
			return nil, fmt.Errorf("line %d: expected = after the key", sc.line(j))
		}
		if sc.i, err = sc.value(sc.skipSpace(j + 1)); err != nil {
			return nil, err
		}
	}
}
