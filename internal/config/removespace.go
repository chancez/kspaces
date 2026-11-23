package config

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Removing a space takes it out of the config as text, as AddSpace puts one in: its spaces entry goes with
// the comments that are its own, and its [space.NAME] tables with theirs, and the rest of the file stays
// byte for byte. The repository is not touched.

// RemoveSpaceText is text without the spaces entry of root and without the [space.NAME] tables of the
// space it names. The result is parsed back and must list every other space as before.
func RemoveSpaceText(text, root string) (string, error) {
	return removeSpaceText(text, root, func(text string) ([]string, error) {
		p, err := decode(text)
		return p.cfg.Spaces, err
	})
}

func removeSpaceText(text, root string, spacesOf func(string) ([]string, error)) (string, error) {
	before, err := spacesOf(text)
	if err != nil {
		return "", fmt.Errorf("the config has an error to fix first: %w", err)
	}
	out := text
	if k := slices.Index(before, root); k >= 0 {
		if out, err = dropSpaceEntry(text, k); err != nil {
			return "", err
		}
	}
	if out, err = dropSpaceTables(out, filepath.Base(root)); err != nil {
		return "", err
	}
	after, err := spacesOf(out)
	if err != nil {
		return "", fmt.Errorf("removing %s would break the config, so it was left as is: %w", filepath.Base(root), err)
	}
	if want := slices.DeleteFunc(slices.Clone(before), func(r string) bool { return r == root }); !slices.Equal(after, want) {
		return "", fmt.Errorf("removing %s would list %q, so the config was left as is", filepath.Base(root), after)
	}
	return out, nil
}

// dropSpaceEntry is text without item k of its spaces array: with its lines, its comments above and after
// it, when it has lines of its own, else the value and its comma.
func dropSpaceEntry(text string, k int) (string, error) {
	_, items, entries, err := scanSpaces(text)
	if err != nil {
		return "", err
	}
	if k >= len(items) {
		return "", fmt.Errorf("the spaces list has %d items, not the %d it lists", len(items), k+1)
	}
	e, it := entries[k], items[k]
	if e.lead >= 0 && e.tail >= 0 {
		end := e.tailEnd
		if end < len(text) && text[end] == '\n' {
			end++
		}
		return text[:e.lead] + text[end:], nil
	}
	switch {
	case it.comma >= 0:
		end := it.comma + 1
		for end < len(text) && (text[end] == ' ' || text[end] == '\t') {
			end++
		}
		return text[:it.start] + text[end:], nil
	case k > 0 && items[k-1].comma >= 0:
		return text[:items[k-1].comma] + text[it.end:], nil
	}
	return text[:it.start] + text[it.end:], nil
}

// dropSpaceTables is text without the [space.NAME] tables of name and the tables under them, each from the
// comments right above its header to the next table's, less the blank lines and comments before that, which
// belong to it.
func dropSpaceTables(text, name string) (string, error) {
	for {
		sc := scanner{s: text}
		headers, err := sc.tables()
		if err != nil {
			return "", err
		}
		i := slices.IndexFunc(headers, func(h tableHeader) bool { return len(h.key) >= 2 && h.key[0] == "space" && h.key[1] == name })
		if i < 0 {
			return text, nil
		}
		start := commentsAbove(text, lineStart(text, headers[i].start))
		end := len(text)
		for _, h := range headers[i+1:] {
			if len(h.key) >= 2 && h.key[0] == "space" && h.key[1] == name {
				continue
			}
			end = commentsAbove(text, lineStart(text, h.start))
			break
		}
		for end > start {
			prev := lineStart(text, end-1)
			if strings.TrimSpace(text[prev:end]) != "" {
				break
			}
			end = prev
		}
		// The blank lines before it go too, but one between what was before and what follows.
		for start > 0 && strings.TrimSpace(text[lineStart(text, start-1):start]) == "" {
			start = lineStart(text, start-1)
		}
		if start > 0 && end < len(text) {
			tail := text[end:]
			for strings.HasPrefix(tail, "\n") {
				tail = tail[1:]
			}
			text = text[:start] + "\n" + tail
			continue
		}
		text = text[:start] + text[end:]
	}
}

// SaveRemoveSpace removes the space at root from the config files, see RemoveSpaceText and EditFile: its
// entry from the file that lists it, and its [space.NAME] tables from every file that has one. The files
// together must parse after.
func SaveRemoveSpace(root string) error {
	files, err := readFiles()
	if err != nil {
		return err
	}
	texts := slices.Clone(files.texts)
	listed := false
	for i, text := range texts {
		p, err := decode(text)
		if err != nil {
			return fmt.Errorf("%s: %w", files.paths[i], err)
		}
		listed = listed || slices.Contains(p.cfg.Spaces, root)
		if texts[i], err = RemoveSpaceText(text, root); err != nil {
			return fmt.Errorf("%s: %w", files.paths[i], err)
		}
	}
	if !listed {
		return fmt.Errorf("%s is not a configured space", Tilde(root))
	}
	if _, err := parse(files.paths, texts); err != nil {
		return fmt.Errorf("removing %s would break the config, so it was left as is: %w", filepath.Base(root), err)
	}
	return files.write(texts)
}
