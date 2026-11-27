package spaces

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// An ad hoc space is one made as it is needed, with a name and a directory to start in, and no repository
// or config behind it: `scratch`, `incident-123`. Its host's server keeps it, see docs/server.md, so every
// machine sees it, and its one kitty session, named after it, is opened and shared as a worktree's is. The
// provider's sessions in it are filed in its space of the same name, as kspaces_space says; a space the
// provider has that no configured or ad hoc space is named after is listed as an ad hoc space too.

// AdHocSpace is an ad hoc space: its name, and the directory its windows start in, empty for the home
// directory.
type AdHocSpace struct {
	Name string `json:"name"`
	Dir  string `json:"dir,omitempty"`
}

// adHocName is what an ad hoc space can be called: as cm takes a space's name, letters, digits, `.`, `_`
// and `-`, not `.` or `..`, so its sessions can be filed under it.
var adHocName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// CheckAdHocName says what is wrong with name as an ad hoc space's in cfg: one cm would refuse, default,
// which every host has, or a configured space's.
func CheckAdHocName(cfg config.Config, name string) error {
	switch {
	case !adHocName.MatchString(name) || name == "." || name == "..":
		return fmt.Errorf("%q: a space's name is letters, digits, '.', '_' and '-'", name)
	case name == defaultSpace:
		return fmt.Errorf("%s is every host's own", name)
	}
	for _, root := range cfg.Spaces {
		if filepath.Base(root) == name {
			return fmt.Errorf("%s is a configured space", name)
		}
	}
	return nil
}

// adHocStore is the ad hoc spaces a host keeps, in a file, "" for none, as in tests.
type adHocStore struct {
	mu     sync.Mutex
	path   string
	spaces []AdHocSpace
	read   bool
}

func (s *adHocStore) load() {
	if s.read {
		return
	}
	s.read = true
	if s.path == "" {
		return
	}
	if b, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(b, &s.spaces)
	}
}

// list is the ad hoc spaces, by name.
func (s *adHocStore) list() []AdHocSpace {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	return slices.Clone(s.spaces)
}

// ErrSpaceExists is an ad hoc space made again.
var ErrSpaceExists = errors.New("there is a space of that name")

// add keeps a, refusing one of a name already kept.
func (s *adHocStore) add(a AdHocSpace) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	if slices.ContainsFunc(s.spaces, func(o AdHocSpace) bool { return o.Name == a.Name }) {
		return fmt.Errorf("%s: %w", a.Name, ErrSpaceExists)
	}
	spaces := append(slices.Clone(s.spaces), a)
	slices.SortFunc(spaces, func(x, y AdHocSpace) int { return strings.Compare(x.Name, y.Name) })
	if err := s.write(spaces); err != nil {
		return err
	}
	s.spaces = spaces
	return nil
}

// remove forgets the space name, and says whether it was kept.
func (s *adHocStore) remove(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	spaces := slices.DeleteFunc(slices.Clone(s.spaces), func(o AdHocSpace) bool { return o.Name == name })
	if len(spaces) == len(s.spaces) {
		return false, nil
	}
	if err := s.write(spaces); err != nil {
		return true, err
	}
	s.spaces = spaces
	return true, nil
}

// write replaces the file with spaces, by a rename.
func (s *adHocStore) write(spaces []AdHocSpace) error {
	if s.path == "" {
		return nil
	}
	b, err := json.Marshal(spaces)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", s.path, os.Getpid())
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// endSpaceOnHost ends the provider's sessions in the space name, on the host it runs on, and has the
// provider remove the space, as removing an ad hoc space does there: no window here shows them.
func endSpaceOnHost(cfg config.Config, name string) error {
	if len(cfg.Provider) == 0 {
		return nil
	}
	caps := ProviderCapabilities(cfg.Provider)
	var errs []error
	if caps[CapList] {
		st, err := listSessions(cfg.Provider, nil)
		if err != nil {
			return err
		}
		var in []ProviderSession
		for _, s := range st.Sessions {
			if s.Host == "" && s.SpaceName() == name {
				in = append(in, s)
			}
		}
		errs = append(errs, ApplyFate(cfg.Provider, caps, FateEnd, in))
	}
	if caps[CapRemoveSpace] {
		errs = append(errs, RemoveProviderSpace(cfg.Provider, name))
	}
	return errors.Join(errs...)
}

// withAdHocDirs starts the sessions in no space that are ad hoc spaces in their directories, see
// AdHocSpace, others being one space each of one worktree.
func withAdHocDirs(others []Space, adHoc []AdHocSpace) {
	for i := range others {
		for _, a := range adHoc {
			if others[i].Name == a.Name && len(others[i].Worktrees) > 0 {
				others[i].Worktrees[0].Path = a.Dir
			}
		}
	}
}

// ExpandHome is path with a leading ~/ as the home directory.
func ExpandHome(path string) string { return expandHome(path) }

// OpenAdHoc switches to wt's kitty session from where kspaces runs, opening it if need be, as
// OpenWorktree does: an ad hoc space's, this machine's or another host's.
func OpenAdHoc(cfg config.Config, wt Worktree) error {
	st, err := kitty.LoadState(kitty.SelfWindowID())
	if err != nil {
		return err
	}
	return OpenWorktree(cfg, st, wt, OpenOptions{})
}
