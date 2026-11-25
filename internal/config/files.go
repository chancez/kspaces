package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// The config can be in more than one file: kspaces.toml, then every .toml file in conf.d beside it, such
// as a work.toml kept out of dotfiles that adds a job's spaces and commands. merge says how they add up.

// fragmentsDir is the directory beside kspaces.toml the other config files are in.
const fragmentsDir = "conf.d"

// configFiles are the config files Load reads, in order, and their texts.
type configFiles struct {
	paths []string
	texts []string
	// What reading kspaces.toml failed with when it is not there, which reads as empty.
	missing error
}

// readFiles reads kspaces.toml, then every .toml file in conf.d by name, hidden ones left out. A missing
// kspaces.toml reads as empty, see configFiles.missing. Another .toml file beside kspaces.toml is an
// error, as it was read before conf.d was.
func readFiles() (configFiles, error) {
	path, err := filePath()
	if err != nil {
		return configFiles{}, err
	}
	dir := filepath.Dir(path)
	beside, err := tomlFiles(dir)
	if err != nil {
		return configFiles{}, err
	}
	for _, other := range beside {
		switch name := filepath.Base(other); {
		case name == "config.toml":
			// kspaces.toml's earlier name.
			return configFiles{}, fmt.Errorf("%s: the config is kspaces.toml now; rename config.toml to it", dir)
		case other != path:
			return configFiles{}, fmt.Errorf("%s: move %s into %s, where the other config files are read from", dir, name, fragmentsDir)
		}
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
	fragments, err := tomlFiles(filepath.Join(dir, fragmentsDir))
	if err != nil {
		return configFiles{}, err
	}
	for _, other := range fragments {
		data, err := os.ReadFile(other)
		if err != nil {
			return configFiles{}, fmt.Errorf("reading the config: %w", err)
		}
		files.paths = append(files.paths, other)
		files.texts = append(files.texts, string(data))
	}
	return files, nil
}

// tomlFiles are the .toml files in dir, by name, hidden ones left out; none for a missing dir.
func tomlFiles(dir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.toml"))
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(paths, func(p string) bool { return strings.HasPrefix(filepath.Base(p), ".") }), nil
}

// FileName is the config file at path as errors and forms name it: its path in the config directory, such
// as conf.d/work.toml, or its name outside it.
func FileName(path string) string {
	if main, err := filePath(); err == nil {
		if rel, err := filepath.Rel(filepath.Dir(main), path); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return filepath.Base(path)
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
