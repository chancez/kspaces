package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// The config can be in more than one file: config.toml, and every other .toml file beside it, such as a
// work.toml kept out of dotfiles that adds a job's spaces and commands. merge says how they add up.

// configFiles are the config files Load reads, in order, and their texts.
type configFiles struct {
	paths []string
	texts []string
	// What reading config.toml failed with when it is not there, which reads as empty.
	missing error
}

// readFiles reads config.toml, then every other .toml file in its directory by name, hidden ones left
// out. A missing config.toml reads as empty, see configFiles.missing.
func readFiles() (configFiles, error) {
	path, err := filePath()
	if err != nil {
		return configFiles{}, err
	}
	var files configFiles
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		files.missing = err
	case err != nil:
		return configFiles{}, fmt.Errorf("reading the config: %w", err)
	}
	files.paths, files.texts = []string{path}, []string{string(data)}
	others, err := filepath.Glob(filepath.Join(filepath.Dir(path), "*.toml"))
	if err != nil {
		return configFiles{}, err
	}
	for _, other := range others {
		if other == path || filepath.Base(other)[0] == '.' {
			continue
		}
		data, err := os.ReadFile(other)
		if err != nil {
			return configFiles{}, fmt.Errorf("reading the config: %w", err)
		}
		files.paths = append(files.paths, other)
		files.texts = append(files.texts, string(data))
	}
	return files, nil
}

// with is the config the files make with file i's text as text, and file i as decode reads it, for an
// edit of one file to be checked merged with the others.
func (f configFiles) with(i int, text string) (Config, decoded, error) {
	texts := slices.Clone(f.texts)
	texts[i] = text
	cfg, err := parse(f.paths, texts)
	if err != nil {
		return Config{}, decoded{}, err
	}
	p, err := decode(text)
	return cfg, p, err
}
