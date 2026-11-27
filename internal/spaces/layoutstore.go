package spaces

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// A host's server keeps the layouts of its copies, see docs/server.md: it is the only writer of each, so
// whichever machine shows a copy saves its changes there, at the revision it last had.

// CopyLayout is a copy's layout as its host keeps it: by Key, the copy's kitty session as the host names
// it, at Revision, 0 for none, with Layout, nil for none.
type CopyLayout struct {
	Key      string        `json:"key"`
	Revision int64         `json:"revision"`
	Layout   *SharedLayout `json:"layout,omitempty"`
}

// ErrLayoutConflict is a save at a revision the layout is no longer at: another machine saved since.
var ErrLayoutConflict = errors.New("the layout was saved by another machine since")

// layoutStore is the layouts a host keeps, in a file, "" for none, as in tests.
type layoutStore struct {
	mu      sync.Mutex
	path    string
	layouts map[string]CopyLayout
}

// read is the store's file, read once.
func (s *layoutStore) read() {
	if s.layouts != nil {
		return
	}
	s.layouts = map[string]CopyLayout{}
	if s.path == "" {
		return
	}
	if b, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(b, &s.layouts)
	}
}

// changed are the layouts of known the store has at another revision than known gives: one saved since,
// or one there is none of, at revision 0.
func (s *layoutStore) changed(known []CopyLayout) []CopyLayout {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.read()
	var out []CopyLayout
	for _, k := range known {
		have := s.layouts[k.Key]
		have.Key = k.Key
		if have.Revision != k.Revision {
			out = append(out, have)
		}
	}
	return out
}

// save makes l the layout of key, if it is still at revision, and is its new revision. ErrLayoutConflict
// when it is not.
func (s *layoutStore) save(key string, revision int64, l SharedLayout) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.read()
	have := s.layouts[key]
	if have.Revision != revision {
		return 0, ErrLayoutConflict
	}
	next := CopyLayout{Key: key, Revision: have.Revision + 1, Layout: &l}
	s.layouts[key] = next
	if err := s.write(); err != nil {
		s.layouts[key] = have
		return 0, err
	}
	return next.Revision, nil
}

// write replaces the file with the layouts, by a rename.
func (s *layoutStore) write() error {
	if s.path == "" {
		return nil
	}
	b, err := json.Marshal(s.layouts)
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
