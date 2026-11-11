// Package gitstate is what git and GitHub say of a worktree beyond its branch: the files it has not
// committed, the commits it is ahead of and behind its upstream, and the pull request of its branch. A
// Watcher keeps them, asking git and gh again only now and then.
package gitstate

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
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

// PR is the pull request of a branch.
type PR struct {
	Number int `json:"number"`
	// State is open, draft, merged or closed.
	State string `json:"state"`
}

// State is what is known of a worktree.
type State struct {
	// Status is nil until git status has answered.
	Status *Status `json:"status,omitempty"`
	// PR is nil when the branch has none, or gh could not say.
	PR *PR `json:"pr,omitempty"`
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

// ghTimeout is how long gh may take, as it goes over the network.
const ghTimeout = 30 * time.Second

// LookupPR asks gh for the pull request of branch in the repository at root, nil when there is none. The
// repository's default branch is left out without asking: pull requests from forks are often named after
// it.
func LookupPR(root, branch string) (*PR, error) {
	if branch == "" || branch == defaultBranch(root) {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "list", "--state", "all", "--head", branch, "--limit", "10",
		"--json", "number,state,isDraft,isCrossRepository")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return ParsePRs(out)
}

// defaultBranch is the branch origin's HEAD points at, else main or master, whichever exists, as named
// without the remote.
func defaultBranch(root string) string {
	out, err := exec.Command("git", "-C", root, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD").Output()
	if err == nil {
		_, name, _ := strings.Cut(strings.TrimSpace(string(out)), "/")
		return name
	}
	for _, name := range []string{"main", "master"} {
		if exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/"+name).Run() == nil {
			return name
		}
	}
	return ""
}

// ParsePRs reads `gh pr list --json number,state,isDraft,isCrossRepository`, newest first, and picks the
// newest from the repository itself over one from a fork of the same name.
func ParsePRs(data []byte) (*PR, error) {
	var prs []struct {
		Number            int    `json:"number"`
		State             string `json:"state"`
		IsDraft           bool   `json:"isDraft"`
		IsCrossRepository bool   `json:"isCrossRepository"`
	}
	if err := json.Unmarshal(data, &prs); err != nil {
		return nil, fmt.Errorf("parsing gh pr list: %w", err)
	}
	pick := -1
	for i, pr := range prs {
		if !pr.IsCrossRepository {
			pick = i
			break
		}
		if pick < 0 {
			pick = i
		}
	}
	if pick < 0 {
		return nil, nil
	}
	pr := prs[pick]
	state := strings.ToLower(pr.State)
	if state == "open" && pr.IsDraft {
		state = "draft"
	}
	return &PR{Number: pr.Number, State: state}, nil
}
