package config

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/chancez/kspaces/internal/logs"
)

// Dragging a space into another group rewrites its entry with the group, as text, as reordering does, and
// moves the entry, with its comments, to the file the group's spaces are in.

// MoveSpace puts the space at root in group, the name of a group or empty for none, and the spaces in
// order, as SaveSpacesOrder does: its entry is rewritten with the group, see withGroup, and goes to the
// file of the group's spaces, see groupFile, when that is another. A group a [space.NAME] table says is
// that table's to change, so moving the space out of it is an error saying so.
func MoveSpace(root, group string, order []string) (err error) {
	defer func(start time.Time) { logs.Action("move space", start, err, "root", root, "group", group) }(time.Now())
	files, cfg, err := readMerged()
	if err != nil {
		return err
	}
	if !slices.Contains(cfg.Spaces, root) {
		return errSpacesChanged
	}
	name := filepath.Base(root)
	if cfg.Space[name].Group == group {
		texts, err := orderedTexts(files.paths, files.texts, order)
		if err != nil {
			return err
		}
		return files.write(texts)
	}
	from, k := -1, -1
	for i, text := range files.texts {
		p, err := decode(text)
		if err != nil {
			return fmt.Errorf("%s: %w", files.paths[i], err)
		}
		if p.md.IsDefined("space", name, "group") {
			return fmt.Errorf("%s's group is set in [space.%s] in %s; change it there", name, name, filepath.Base(files.paths[i]))
		}
		if j := slices.Index(p.cfg.Spaces, root); j >= 0 && from < 0 {
			from, k = i, j
		}
	}
	text := files.texts[from]
	_, items, _, err := scanSpaces(text)
	if err != nil {
		return fmt.Errorf("%s: %w", files.paths[from], err)
	}
	it := items[k]
	moved, err := withGroup(text[it.start:it.end], group)
	if err != nil {
		return fmt.Errorf("%s: %w", files.paths[from], err)
	}
	to, err := groupFile(files, cfg, group, root)
	if err != nil {
		return err
	}
	texts := slices.Clone(files.texts)
	if to == from {
		texts[from] = text[:it.start] + moved + text[it.end:]
	} else {
		if texts[from], err = removeItem(text, k); err != nil {
			return fmt.Errorf("%s: %w", files.paths[from], err)
		}
		if texts[to], err = appendSpace(texts[to], moved); err != nil {
			return fmt.Errorf("%s: %w", files.paths[to], err)
		}
	}
	if texts, err = orderedTexts(files.paths, texts, order); err != nil {
		return err
	}
	after, err := parse(files.paths, texts)
	if err != nil {
		return fmt.Errorf("moving %s would break the config, so it was left as is: %w", name, err)
	}
	if g := after.Space[name].Group; g != group {
		return fmt.Errorf("moving %s would put it in group %q, so the config was left as is", name, g)
	}
	return files.write(texts)
}

// withGroup is v, an entry of spaces, in group: a path made a table with the group, a table's group set
// or added, or, for none, left out, a table left with its path alone being the path again.
func withGroup(v, group string) (string, error) {
	if !strings.HasPrefix(v, "{") {
		if group == "" {
			return v, nil
		}
		return "{ path = " + v + ", group = " + tomlString(group) + " }", nil
	}
	pairs, err := inlinePairs(v)
	if err != nil {
		return "", err
	}
	if len(pairs) == 0 {
		return "", fmt.Errorf("%s has no path", v)
	}
	gi := slices.IndexFunc(pairs, func(p tablePair) bool { return p.key == "group" })
	switch {
	case group != "" && gi >= 0:
		p := pairs[gi]
		return v[:p.valStart] + tomlString(group) + v[p.valEnd:], nil
	case group != "":
		last := pairs[len(pairs)-1]
		return v[:last.valEnd] + ", group = " + tomlString(group) + v[last.valEnd:], nil
	case gi < 0:
		return v, nil
	}
	if len(pairs) == 2 {
		if path := pairs[1-gi]; path.key == "path" {
			return v[path.valStart:path.valEnd], nil
		}
	}
	p := pairs[gi]
	switch {
	case gi+1 < len(pairs):
		return v[:p.start] + v[pairs[gi+1].start:], nil
	case gi > 0:
		return v[:pairs[gi-1].valEnd] + v[p.valEnd:], nil
	}
	return v[:p.start] + v[p.valEnd:], nil
}

// tablePair is a key = value of an inline table: where it starts, its key, and where its value is.
type tablePair struct {
	start    int
	key      string
	valStart int
	valEnd   int
}

// inlinePairs are the pairs of the inline table t, a single key each, in order.
func inlinePairs(t string) ([]tablePair, error) {
	sc := scanner{s: t}
	var pairs []tablePair
	for i := 1; ; {
		i = sc.skipBlank(i)
		switch {
		case i >= len(t):
			return nil, fmt.Errorf("%s does not end", t)
		case t[i] == '}':
			return pairs, nil
		}
		key, j, err := sc.key(i)
		if err != nil {
			return nil, err
		}
		j = sc.skipSpace(j)
		if j >= len(t) || t[j] != '=' {
			return nil, fmt.Errorf("%s: expected = after %s", t, strings.Join(key, "."))
		}
		start := sc.skipSpace(j + 1)
		end, err := sc.value(start)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, tablePair{start: i, key: strings.Join(key, "."), valStart: start, valEnd: end})
		i = sc.skipBlank(end)
		if i < len(t) && t[i] == ',' {
			i++
		}
	}
}

// removeItem is text with item k of its spaces array left out: with its line, and the comment lines
// above it, when it has a line of its own, else with its comma.
func removeItem(text string, k int) (string, error) {
	_, items, entries, err := scanSpaces(text)
	if err != nil {
		return "", err
	}
	it, e := items[k], entries[k]
	if e.lead >= 0 && e.tail >= 0 {
		end := e.tailEnd
		if end < len(text) && text[end] == '\n' {
			end++
		}
		return text[:e.lead] + text[end:], nil
	}
	switch {
	case it.comma >= 0 && k+1 < len(items):
		return text[:it.start] + text[items[k+1].start:], nil
	case it.comma >= 0:
		return text[:it.start] + strings.TrimLeft(text[it.comma+1:], " "), nil
	case k > 0:
		return text[:items[k-1].end] + text[it.end:], nil
	}
	return text[:it.start] + text[it.end:], nil
}
