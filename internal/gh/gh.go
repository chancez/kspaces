// Package gh asks GitHub's gh about pull requests.
package gh

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// PR is the part of `gh pr view --json` kspaces checks a pull request out from.
type PR struct {
	Number int `json:"number"`
	// The branch the PR is from, in the repository it is from.
	HeadRefName         string `json:"headRefName"`
	HeadRepositoryOwner Owner  `json:"headRepositoryOwner"`
	// Set when the PR is from another repository than the one it is to, such as a fork.
	IsCrossRepository bool   `json:"isCrossRepository"`
	Title             string `json:"title"`
	// The PR's page, which names the repository it is to, for picking the remote to fetch it from.
	URL string `json:"url"`
}

// Owner is the user or organization a repository belongs to.
type Owner struct {
	Login string `json:"login"`
}

// fields are what ViewPR asks gh for, PR's.
const fields = "number,headRefName,headRepositoryOwner,isCrossRepository,title,url"

// ViewPR looks up the PR pr, a number or a URL, with gh run in dir, so a number is one of the PRs of the
// GitHub repository dir's remotes point to. The error is gh's own message, such as for a PR that does not
// exist or a repository not on GitHub.
func ViewPR(dir, pr string) (PR, error) {
	cmd := exec.Command("gh", "pr", "view", pr, "--json", fields)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return PR{}, ghError(err, "looking up a PR", "gh pr view")
	}
	var ans PR
	if err := json.Unmarshal(out, &ans); err != nil {
		return PR{}, fmt.Errorf("parsing gh pr view: %w", err)
	}
	return ans, nil
}

// Browse opens PR number in the browser, with gh run in dir for the repository it is of. The error is
// gh's own message, as ViewPR's.
func Browse(dir string, number int) error {
	cmd := exec.Command("gh", "browse", strconv.Itoa(number))
	cmd.Dir = dir
	if _, err := cmd.Output(); err != nil {
		return ghError(err, "opening a PR", "gh browse")
	}
	return nil
}

// ghError is err from running gh, for doing what, as gh's message when it said one.
func ghError(err error, doing, name string) error {
	var ee *exec.ExitError
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return fmt.Errorf("gh is not installed, and %s needs it", doing)
	case errors.As(err, &ee) && len(ee.Stderr) > 0:
		return errors.New(strings.TrimSpace(string(ee.Stderr)))
	}
	return fmt.Errorf("%s: %w", name, err)
}
