// Package gh is how kspaces asks GitHub, through GitHub's gh: about pull requests, and to open one in the
// browser. gh runs in a repository's directory, so it works out the GitHub repository from the remotes.
package gh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// PRView is what ViewPR says of a PR, as much as checking it out needs.
type PRView struct {
	Number int
	Title  string
	// The branch the PR is from, in the repository it is from, and that repository's owner.
	HeadRef, Owner string
	// From another repository than the one it is to, such as a fork.
	Cross bool
	// The PR's page, which names the repository it is to, for picking the remote to fetch it from.
	URL string
}

// viewFields are what ViewPR asks gh for, PRView's.
const viewFields = "number,headRefName,headRepositoryOwner,isCrossRepository,title,url"

// ViewPR looks up the PR pr, a number or a URL, with gh run in dir, so a number is one of the PRs of the
// GitHub repository dir's remotes point to. The error is gh's own message, such as for a PR that does not
// exist or a repository not on GitHub.
func ViewPR(dir, pr string) (PRView, error) {
	out, err := run(context.Background(), dir, "looking up a PR", "pr", "view", pr, "--json", viewFields)
	if err != nil {
		return PRView{}, err
	}
	var got struct {
		Number              int    `json:"number"`
		HeadRefName         string `json:"headRefName"`
		HeadRepositoryOwner struct {
			Login string `json:"login"`
		} `json:"headRepositoryOwner"`
		IsCrossRepository bool   `json:"isCrossRepository"`
		Title             string `json:"title"`
		URL               string `json:"url"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		return PRView{}, fmt.Errorf("parsing gh pr view: %w", err)
	}
	return PRView{
		Number:  got.Number,
		Title:   got.Title,
		HeadRef: got.HeadRefName,
		Owner:   got.HeadRepositoryOwner.Login,
		Cross:   got.IsCrossRepository,
		URL:     got.URL,
	}, nil
}

// Browse opens PR number in the browser, with gh run in dir for the repository it is of. The error is
// gh's own message, as ViewPR's. It has no timeout, as gh may wait on the browser it starts.
func Browse(dir string, number int) error {
	_, err := run(context.Background(), dir, "opening a PR", "browse", strconv.Itoa(number))
	return err
}

// run runs gh with args in dir, for doing what, until ctx is done, and returns what it printed. The error
// is gh's message when it said one.
func run(ctx context.Context, dir, doing string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return out, nil
	case errors.Is(err, exec.ErrNotFound):
		return nil, fmt.Errorf("gh is not installed, and %s needs it", doing)
	case errors.As(err, &ee) && len(ee.Stderr) > 0:
		return nil, errors.New(strings.TrimSpace(string(ee.Stderr)))
	}
	name := "gh " + args[0]
	if args[0] == "pr" {
		name += " " + args[1]
	}
	return nil, fmt.Errorf("%s: %w", name, err)
}
