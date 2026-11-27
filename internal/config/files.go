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
// as a work.toml kept out of dotfiles that adds a job's spaces and commands, then the overlays beside it:
// kspaces.ENV.toml for each environment, see Envs, such as a host's spaces kept in dotfiles but listed on
// that host alone, and kspaces.local.toml, kept out of dotfiles. merge says how they add up.

// fragmentsDir is the directory beside kspaces.toml the other config files are in.
const fragmentsDir = "conf.d"

// EnvVar names the environments whose overlays are read, comma separated, a later one's taking the place
// of an earlier one's. Unset, the environment is the host's name.
const EnvVar = "KSPACES_ENV"

// localEnv is the overlay read last, on every machine.
const localEnv = "local"

// Envs are the environments whose overlays are read, in order, see EnvVar: its names, or the host's name,
// up to its first dot, when it is unset. local is always read last, so it is not one of them.
func Envs() ([]string, error) {
	v, ok := os.LookupEnv(EnvVar)
	if !ok {
		host, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("finding the host's name for the config: %w", err)
		}
		v, _, _ = strings.Cut(host, ".")
	}
	var envs []string
	for env := range strings.SplitSeq(v, ",") {
		switch env = strings.TrimSpace(env); {
		case env == "" || env == localEnv || slices.Contains(envs, env):
		case strings.ContainsAny(env, `/\`) || strings.HasPrefix(env, "."):
			return nil, fmt.Errorf("%s: %q cannot name a config file", EnvVar, env)
		default:
			envs = append(envs, env)
		}
	}
	return envs, nil
}

// isOverlay says whether the config file called name, see FileName, is an overlay: kspaces.ENV.toml.
func isOverlay(name string) bool {
	env, ok := strings.CutPrefix(name, "kspaces.")
	env, toml := strings.CutSuffix(env, ".toml")
	return ok && toml && env != "" && !strings.Contains(name, "/")
}

// configFiles are the config files Load reads, in order, and their texts.
type configFiles struct {
	paths []string
	texts []string
	// What reading kspaces.toml failed with when it is not there, which reads as empty.
	missing error
}

// readFiles reads kspaces.toml, then every .toml file in conf.d by name, hidden ones left out, then the
// overlays there are of Envs, in order, and kspaces.local.toml. A missing kspaces.toml reads as empty, see
// configFiles.missing. Another .toml file beside kspaces.toml than an overlay is an error, as it was read
// before conf.d was.
func readFiles() (configFiles, error) {
	path, err := FilePath()
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
		case other != path && !isOverlay(name):
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
	envs, err := Envs()
	if err != nil {
		return configFiles{}, err
	}
	for _, env := range append(envs, localEnv) {
		if overlay := filepath.Join(dir, "kspaces."+env+".toml"); slices.Contains(beside, overlay) {
			fragments = append(fragments, overlay)
		}
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
	if path == "" {
		return ""
	}
	if main, err := FilePath(); err == nil {
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
