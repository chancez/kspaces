package config

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
)

// Moving a space in the sidebar reorders the config's spaces as text, as AddSpace adds one: each entry
// moves with its comments, and the array keeps its layout, its commas and the comments that belong to no
// entry where they were.

// ReorderSpaces is the config text with its spaces listed in order, which must be the roots it lists, as
// Parse reads them, in another order. The result is parsed back and must list them in that order.
func ReorderSpaces(text string, order []string) (string, error) {
	return reorderSpaces(text, order, func(text string) ([]string, error) {
		cfg, err := Parse(text)
		return cfg.Spaces, err
	})
}

var errSpacesChanged = errors.New("the spaces have changed since the sidebar read them")

// reorderSpaces is ReorderSpaces with spacesOf reading the spaces a text lists.
func reorderSpaces(text string, order []string, spacesOf func(string) ([]string, error)) (string, error) {
	before, err := spacesOf(text)
	if err != nil {
		return "", fmt.Errorf("the config has an error to fix first: %w", err)
	}
	if !slices.Equal(slices.Sorted(slices.Values(before)), slices.Sorted(slices.Values(order))) {
		return "", errSpacesChanged
	}
	if slices.Equal(before, order) {
		return text, nil
	}
	// The entry of before that goes in each place, the first not yet placed of equal ones.
	from, used := make([]int, len(order)), make([]bool, len(order))
	for j, root := range order {
		for i, r := range before {
			if r == root && !used[i] {
				from[j], used[i] = i, true
				break
			}
		}
	}
	out, err := reorderItems(text, from)
	if err != nil {
		return "", err
	}
	after, err := spacesOf(out)
	if err != nil {
		return "", fmt.Errorf("reordering the spaces would break the config, so it was left as is: %w", err)
	}
	if !slices.Equal(after, order) {
		return "", fmt.Errorf("reordering the spaces would list %q, so the config was left as is", after)
	}
	return out, nil
}

// SaveSpacesOrder reorders the spaces of the config files, see ReorderSpaces and EditFile: each file's
// among the places it lists them in. The sidebar lists a group's spaces as the files do, one file's after
// another's, so an order that puts a space before one of a file before its own is an error, as the files
// cannot say it.
func SaveSpacesOrder(order []string) error {
	files, err := readFiles()
	if err != nil {
		return err
	}
	before, err := parse(files.paths, files.texts)
	if err != nil {
		return fmt.Errorf("the config has an error to fix first: %w", err)
	}
	if !slices.Equal(slices.Sorted(slices.Values(before.Spaces)), slices.Sorted(slices.Values(order))) {
		return errSpacesChanged
	}
	// Each file's spaces in order, a root listed twice taken by the first files to list it.
	left := make([]map[string]int, len(files.texts))
	for i, text := range files.texts {
		p, err := decode(text)
		if err != nil {
			return err
		}
		left[i] = map[string]int{}
		for _, root := range p.cfg.Spaces {
			left[i][root]++
		}
	}
	own, fileOf := make([][]string, len(files.texts)), map[string]int{}
	for _, root := range order {
		i := slices.IndexFunc(left, func(n map[string]int) bool { return n[root] > 0 })
		left[i][root]--
		own[i] = append(own[i], root)
		if _, ok := fileOf[root]; !ok {
			fileOf[root] = i
		}
	}
	texts := slices.Clone(files.texts)
	for i, text := range files.texts {
		texts[i], err = reorderSpaces(text, own[i], func(text string) ([]string, error) {
			p, err := decode(text)
			return p.cfg.Spaces, err
		})
		if err != nil {
			return fmt.Errorf("%s: %w", files.paths[i], err)
		}
	}
	after, err := parse(files.paths, texts)
	if err != nil {
		return fmt.Errorf("reordering the spaces would break the config, so it was left as is: %w", err)
	}
	group := func(roots []string) map[string][]string {
		ans := map[string][]string{}
		for _, root := range roots {
			g := after.Space[filepath.Base(root)].Group
			ans[g] = append(ans[g], root)
		}
		return ans
	}
	want, got := group(order), group(after.Spaces)
	for _, g := range slices.Sorted(maps.Keys(want)) {
		for k, a := range want[g] {
			if b := got[g][k]; a != b {
				return fmt.Errorf("%s cannot go before %s: it is in %s, whose spaces come after %s's", filepath.Base(a), filepath.Base(b),
					filepath.Base(files.paths[fileOf[a]]), filepath.Base(files.paths[fileOf[b]]))
			}
		}
	}
	for i := range texts {
		if texts[i] == files.texts[i] {
			continue
		}
		err := EditFile(files.paths[i], func(text string) (string, error) {
			if text != files.texts[i] {
				return "", errSpacesChanged
			}
			return texts[i], nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// spaceEntry is an item of the spaces array with what moves with it, each a range of the text: the
// comment lines right above it, its value, and the rest of its line after its comma, a comment or
// nothing. lead is -1 when the item is not the first on its line, and tail -1 when it is not the last, as
// there is no line of its own to keep comments on.
type spaceEntry struct {
	lead, leadEnd int
	start, end    int
	tail, tailEnd int
}

// reorderItems is text with the items of the spaces array moved, item from[j] to the place of item j.
// Each place keeps its indentation, comma and line breaks, and takes the moved item's value and comments.
func reorderItems(text string, from []int) (string, error) {
	sc := scanner{s: text}
	open, _, err := sc.spacesArray()
	if err != nil {
		return "", err
	}
	if open < 0 {
		return "", fmt.Errorf("the config lists no spaces")
	}
	items, _, err := sc.items(open)
	if err != nil {
		return "", err
	}
	if len(items) != len(from) {
		return "", fmt.Errorf("the spaces list has %d items, not the %d spaces it lists", len(items), len(from))
	}
	entries := make([]spaceEntry, len(items))
	bound := open + 1
	for k, it := range items {
		e := spaceEntry{lead: -1, start: it.start, end: it.end, tail: -1}
		if ls := lineStart(text, it.start); ls >= bound && strings.TrimSpace(text[ls:it.start]) == "" {
			e.lead, e.leadEnd = ls, ls
			for e.lead > bound {
				prev := lineStart(text, e.lead-1)
				if prev < bound || !strings.HasPrefix(strings.TrimLeft(text[prev:e.lead-1], " \t"), "#") {
					break
				}
				e.lead = prev
			}
		}
		after := it.end
		if it.comma >= 0 {
			after = it.comma + 1
		}
		nl := strings.IndexByte(text[after:], '\n')
		if nl < 0 {
			nl = len(text) - after
		}
		if rest := strings.TrimLeft(text[after:after+nl], " \t\r"); rest == "" || rest[0] == '#' {
			e.tail, e.tailEnd = after, after+nl
			bound = after + nl
		} else {
			bound = after
		}
		entries[k] = e
	}
	part := func(i, j int) string {
		if i < 0 {
			return ""
		}
		return text[i:j]
	}
	var b strings.Builder
	at := 0
	for j, place := range entries {
		e := entries[from[j]]
		lead, tail := part(e.lead, e.leadEnd), part(e.tail, e.tailEnd)
		if place.lead < 0 && lead != "" || place.tail < 0 && strings.TrimSpace(tail) != "" {
			return "", fmt.Errorf("line %d: the comments of %s have no line of their own to move with it", sc.line(e.start), text[e.start:e.end])
		}
		if place.lead >= 0 {
			b.WriteString(text[at:place.lead])
			b.WriteString(lead)
			at = place.leadEnd
		}
		b.WriteString(text[at:place.start])
		b.WriteString(text[e.start:e.end])
		at = place.end
		if place.tail >= 0 {
			b.WriteString(text[at:place.tail])
			b.WriteString(tail)
			at = place.tailEnd
		}
	}
	b.WriteString(text[at:])
	return b.String(), nil
}
