// Package gitstate is what git and GitHub say of a worktree beyond its branch: the files it has not
// committed, the commits it is ahead of and behind its upstream, and the pull request of its branch. A
// Watcher keeps them, asking git and, through package gh, GitHub again only now and then.
package gitstate

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/chancez/kspaces/internal/gh"
)

// Status is what `git status` says of a worktree.
type Status struct {
	// Changes is how many files are changed, staged or untracked.
	Changes int `json:"changes,omitempty"`
	// Upstream is the branch's upstream, such as origin/main, empty for none.
	Upstream string `json:"upstream,omitempty"`
	Ahead    int    `json:"ahead,omitempty"`
	Behind   int    `json:"behind,omitempty"`
	// Gone is an upstream that is set but no longer exists, as after its branch was deleted on merge.
	Gone bool `json:"gone,omitempty"`
}

// State is what is known of a worktree.
type State struct {
	// Status is nil until git status has answered.
	Status *Status `json:"status,omitempty"`
	// PR is nil when the branch has none, or gh could not say.
	PR *gh.PR `json:"pr,omitempty"`
	// When gh was last asked, and for which branch.
	PRAt     time.Time `json:"pr_at,omitzero"`
	PRBranch string    `json:"pr_branch,omitempty"`
}

// ReadStatus runs git status in the worktree at dir. It takes no optional locks, so running it in the
// background never gets in the way of git run by hand.
func ReadStatus(dir string) (Status, error) {
	out, err := exec.Command("git", "--no-optional-locks", "-C", dir, "status", "--porcelain=v2", "--branch").Output()
	if err != nil {
		return Status{}, err
	}
	return ParseStatus(string(out)), nil
}

// ParseStatus reads `git status --porcelain=v2 --branch`. An upstream with no ahead and behind line is one
// that is gone.
func ParseStatus(porcelain string) Status {
	var s Status
	ab := false
	sc := bufio.NewScanner(strings.NewReader(porcelain))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
		case strings.HasPrefix(line, "# branch.upstream "):
			s.Upstream = strings.TrimPrefix(line, "# branch.upstream ")
		case strings.HasPrefix(line, "# branch.ab "):
			ab = true
			fmt.Sscanf(strings.TrimPrefix(line, "# branch.ab "), "+%d -%d", &s.Ahead, &s.Behind)
		case strings.HasPrefix(line, "#"):
		default:
			s.Changes++
		}
	}
	s.Gone = s.Upstream != "" && !ab
	return s
}
