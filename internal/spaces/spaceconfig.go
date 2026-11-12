package spaces

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chancez/kspaces/internal/config"
)

// A space's .kspaces.toml is a repository's own file, often committed, so a clone, a pull or a branch
// checked out can bring one: it runs commands, so it is used only once trusted, for the content trusted.

// ConfigPath is where the space's file is: in its main worktree, which every linked one shares it from.
func (s Space) ConfigPath() string {
	return filepath.Join(s.Root, config.SpaceFileName)
}

// ConfigOf is the config as s sees it: its own when its file is in use, else global.
func (s Space) ConfigOf(global config.Config) config.Config {
	if s.Config != nil {
		return *s.Config
	}
	return global
}

// readConfig reads s's file, if it has one, into s: its config merged with global's when trusted and good,
// whether it is untrusted, or what is wrong with it.
func (s *Space) readConfig(global config.Config) {
	if s.Err != nil {
		return
	}
	path := s.ConfigPath()
	sf, err := readSpaceFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return
	case err != nil:
		s.ConfigErr = fmt.Errorf("%s: %w", config.Tilde(path), err)
		return
	case !Trusted(path, sf.data):
		s.Untrusted = true
		return
	case sf.err != nil:
		s.ConfigErr = fmt.Errorf("%s: %w", config.Tilde(path), sf.err)
		return
	}
	cfg, err := global.WithSpace(s.Name, sf.file)
	if err != nil {
		s.ConfigErr = fmt.Errorf("%s: %w", config.Tilde(path), err)
		return
	}
	s.Config = &cfg
}

// ReadSpace is the space at root, LoadSpace's, with its file read as cfg's, see Space.Config.
func ReadSpace(cfg config.Config, root string) Space {
	s := LoadSpace(root)
	s.readConfig(cfg)
	return s
}

// spaceFile is a space file as it was read: its content and what parsing it said.
type spaceFile struct {
	data []byte
	file config.SpaceFile
	err  error
}

// stamp is what a file's stat says of its content changing: a rewrite changes its size or its time, and
// a rename into its place, as an atomic write does, its inode too.
type stamp struct {
	size  int64
	mtime time.Time
	inode uint64
}

func stampOf(info fs.FileInfo) stamp {
	st := stamp{size: info.Size(), mtime: info.ModTime()}
	if sys, ok := info.Sys().(*syscall.Stat_t); ok {
		st.inode = sys.Ino
	}
	return st
}

// statCache keeps what was made of each file it has read, by path, so the sidebars' poll every two seconds
// stats the space files and the trust file rather than reading and parsing them again.
type statCache[T any] struct {
	mu      sync.Mutex
	entries map[string]statEntry[T]
}

type statEntry[T any] struct {
	stamp stamp
	value T
}

// get is what read made of the file at path, again only when its stat says it changed. A missing file is
// fs.ErrNotExist, and not kept.
func (c *statCache[T]) get(path string, read func(data []byte) T) (T, error) {
	var zero T
	info, err := os.Stat(path)
	if err != nil {
		c.mu.Lock()
		delete(c.entries, path)
		c.mu.Unlock()
		return zero, err
	}
	st := stampOf(info)
	c.mu.Lock()
	e, ok := c.entries[path]
	c.mu.Unlock()
	if ok && e.stamp == st {
		return e.value, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return zero, err
	}
	v := read(data)
	c.mu.Lock()
	if c.entries == nil {
		c.entries = map[string]statEntry[T]{}
	}
	c.entries[path] = statEntry[T]{st, v}
	c.mu.Unlock()
	return v, nil
}

var spaceFiles statCache[spaceFile]

func readSpaceFile(path string) (spaceFile, error) {
	return spaceFiles.get(path, func(data []byte) spaceFile {
		f, err := config.ParseSpaceFile(string(data))
		return spaceFile{data: data, file: f, err: err}
	})
}

// The space files trusted, a line each: the sha256 of the content trusted, two spaces and the file's
// absolute path, as sha256sum writes them.
const trustFile = "trust"

var trusts statCache[map[string]string]

// loadTrust is the trusted hashes by path.
func loadTrust() map[string]string {
	path, err := statePath(trustFile)
	if err != nil {
		return nil
	}
	ans, _ := trusts.get(path, func(data []byte) map[string]string {
		ans := map[string]string{}
		for line := range strings.Lines(string(data)) {
			if sum, file, ok := strings.Cut(strings.TrimSuffix(line, "\n"), "  "); ok {
				ans[file] = sum
			}
		}
		return ans
	})
	return ans
}

func contentSum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Trusted says whether data is the content trusted for the space file at path.
func Trusted(path string, data []byte) bool {
	sum, ok := loadTrust()[path]
	return ok && sum == contentSum(data)
}

// Trust trusts data as the content of the space file at path, in place of any it trusted before.
func Trust(path string, data []byte) error {
	return editTrust(func(t map[string]string) { t[path] = contentSum(data) })
}

// Revoke forgets the space file at path, so it is untrusted whatever it has.
func Revoke(path string) error {
	return editTrust(func(t map[string]string) { delete(t, path) })
}

func editTrust(edit func(map[string]string)) error {
	t := maps.Clone(loadTrust())
	if t == nil {
		t = map[string]string{}
	}
	edit(t)
	var b strings.Builder
	for _, path := range slices.Sorted(maps.Keys(t)) {
		fmt.Fprintf(&b, "%s  %s\n", t[path], path)
	}
	return writeState(trustFile, []byte(b.String()))
}
