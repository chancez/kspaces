package config

import (
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

// spaceEntries is the spaces list as config.toml has it: each entry a path, or an inline table of a path
// and what a [space.NAME] table says of the space, which reads better than a table each for a setting or
// two:
//
//	spaces = [
//	  { path = "~/projects/gr", group = "work" },
//	  "~/.dotfiles",
//	]
//
// The key is path rather than name: a space's name is its directory's, and name = would read as a name
// to show it by. Parse folds the tables into Config.Space, so Config.Spaces stays the paths.
type spaceEntries []listedSpace

// listedSpace is one entry of spaces: its path, and what its table, if any, says of the space, with the
// keys it sets.
type listedSpace struct {
	path     string
	settings SpaceSettings
	keys     []string
}

// UnmarshalTOML reads the spaces list. A table's keys other than path are read as a [space.NAME] table's,
// and one it does not have is an error.
func (e *spaceEntries) UnmarshalTOML(v any) error {
	items, ok := v.([]any)
	if !ok {
		return fmt.Errorf("spaces must be a list of paths, or of { path = ... } tables")
	}
	for i, item := range items {
		switch item := item.(type) {
		case string:
			*e = append(*e, listedSpace{path: item})
		case map[string]any:
			entry, err := readSpaceTable(item)
			if err != nil {
				return fmt.Errorf("spaces: item %d: %w", i+1, err)
			}
			*e = append(*e, entry)
		default:
			return fmt.Errorf("spaces: item %d is neither a path nor a { path = ... } table", i+1)
		}
	}
	return nil
}

// readSpaceTable reads an inline table of spaces: path, and the rest as the SpaceSettings fields of their
// toml names, of the fields' types, so it takes what a [space.NAME] table takes.
func readSpaceTable(table map[string]any) (listedSpace, error) {
	raw, ok := table["path"]
	if !ok {
		return listedSpace{}, fmt.Errorf("it has no path")
	}
	path, ok := raw.(string)
	switch {
	case !ok:
		return listedSpace{}, fmt.Errorf("path must be a string")
	case path == "":
		return listedSpace{}, fmt.Errorf("path is empty")
	}
	entry := listedSpace{path: path}
	var unknown []string
	for _, key := range slices.Sorted(maps.Keys(table)) {
		if key == "path" {
			continue
		}
		f, ok := spaceField(&entry.settings, key)
		switch v := reflect.ValueOf(table[key]); {
		case !ok:
			unknown = append(unknown, key)
		case !v.Type().AssignableTo(f.Type()):
			return listedSpace{}, fmt.Errorf("%s: %s must be a %s", path, key, f.Type())
		default:
			f.Set(v)
			entry.keys = append(entry.keys, key)
		}
	}
	if len(unknown) > 0 {
		return listedSpace{}, fmt.Errorf("%s: unknown keys: %s", path, strings.Join(unknown, ", "))
	}
	return entry, nil
}

// spaceField is the field of s that key names, by its toml tag.
func spaceField(s *SpaceSettings, key string) (reflect.Value, bool) {
	v := reflect.ValueOf(s).Elem()
	for i := range v.NumField() {
		if v.Type().Field(i).Tag.Get("toml") == key {
			return v.Field(i), true
		}
	}
	return reflect.Value{}, false
}

// mergeSpaceEntries folds what the entries of spaces say of their spaces into cfg.Space, by the spaces'
// names, as [space.NAME] tables would. defined reports whether a [space.NAME] table sets a key. A key set
// in both places, or by two entries named alike, is an error rather than one silently winning.
func mergeSpaceEntries(cfg *Config, entries spaceEntries, defined func(name, key string) bool) error {
	from := map[string]map[string]bool{}
	for _, e := range entries {
		if len(e.keys) == 0 {
			continue
		}
		name := filepath.Base(e.path)
		if from[name] == nil {
			from[name] = map[string]bool{}
		}
		s := cfg.Space[name]
		for _, key := range e.keys {
			switch {
			case defined(name, key):
				return fmt.Errorf("%s's %s is set both in its spaces entry and in [space.%s]; set it in one", name, key, name)
			case from[name][key]:
				return fmt.Errorf("%s's %s is set by two entries of spaces", name, key)
			}
			from[name][key] = true
			dst, _ := spaceField(&s, key)
			src, _ := spaceField(&e.settings, key)
			dst.Set(src)
		}
		if cfg.Space == nil {
			cfg.Space = map[string]SpaceSettings{}
		}
		cfg.Space[name] = s
	}
	return nil
}
