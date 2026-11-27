package spaces

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gittest"
)

// A host's server creates a worktree where the space's config puts it, and removes one ending the
// provider's sessions tagged with it, with no kitty to ask: the windows are another machine's.
func TestHostWorktrees(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	r := gittest.New(t)
	r.Commit("first")
	bin := t.TempDir()
	log := filepath.Join(bin, "log")
	scripts := map[string]string{
		// Any kitty asked fails the test's removal.
		"kitten": "#!/bin/sh\necho kitten \"$*\" >> " + log + "\nexit 1\n",
		"prov": "#!/bin/sh\ncat > /dev/null\ncase $1 in\n" +
			"capabilities) echo '{\"capabilities\": [\"list\", \"kill\", \"remove\"]}' ;;\n" +
			"list) echo '{\"sessions\": [" +
			"{\"id\": \"@a\", \"name\": \"a\", \"space\": \"" + filepath.Base(r.Dir) + "\", \"state\": \"running\", \"tags\": {\"worktree\": \"dev\"}}, " +
			"{\"id\": \"@b\", \"name\": \"b\", \"space\": \"" + filepath.Base(r.Dir) + "\", \"state\": \"running\"}]}' ;;\n" +
			"*) echo \"$1\" >> " + log + " ;;\nesac\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	cfg := config.Config{Spaces: []string{r.Dir}, Provider: []string{filepath.Join(bin, "prov")}, Worktrees: config.Worktrees{Dir: ".worktrees", BranchPrefix: "pr/me"}}

	wt, err := CreateOnHost(cfg, r.Dir, "pr/me/dev", "")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(r.Dir, ".worktrees", "dev")
	if want := (Worktree{Path: dir, Branch: "pr/me/dev", Session: filepath.Base(r.Dir) + "@dev"}); wt != want {
		t.Errorf("created %+v, want %+v", wt, want)
	}
	if err := RemoveOnHost(cfg, dir, RemoveOptions{DeleteBranch: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("%s is still there: %v", dir, err)
	}
	if branches := strings.TrimSpace(r.Git("branch", "--format=%(refname:short)")); branches != "main" {
		t.Errorf("branches %q, want main alone", branches)
	}
	data, _ := os.ReadFile(log)
	// The session tagged with dev was killed; nothing asked kitty.
	if got, want := strings.Fields(string(data)), []string{"kill"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
}
