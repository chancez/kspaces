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
	"time"

	"github.com/chancez/kspaces/internal/git"
)

// PR is the pull request of a branch, as the sidebar shows it.
type PR struct {
	Number int `json:"number"`
	// State is open, draft, merged or closed.
	State  string `json:"state"`
	Checks Checks `json:"checks,omitempty"`
	Review Review `json:"review,omitempty"`
}

// Checks is what a PR's CI checks come to, all of them together. The zero value is none, so a poller
// file written before PRs had checks reads as none.
type Checks string

const (
	ChecksNone    Checks = ""
	ChecksPassing Checks = "passing"
	ChecksFailing Checks = "failing"
	ChecksPending Checks = "pending"
)

// Review is a PR's review decision, as GitHub works it out from the reviews and the branch's rules. The
// zero value is none, as for Checks.
type Review string

const (
	ReviewNone             Review = ""
	ReviewApproved         Review = "approved"
	ReviewChangesRequested Review = "changes_requested"
	ReviewRequired         Review = "review_required"
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

// branchTimeout is how long BranchPR lets gh take, as it goes over the network in the background.
const branchTimeout = 30 * time.Second

// BranchPR asks gh for the pull request of branch in the repository at root, nil when there is none. The
// repository's default branch is left out without asking: pull requests from forks are often named after
// it.
func BranchPR(root, branch string) (*PR, error) {
	if branch == "" || branch == git.DefaultBranch(root) {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), branchTimeout)
	defer cancel()
	out, err := run(ctx, root, "looking up a branch's PR", "pr", "list", "--state", "all", "--head", branch, "--limit", "10",
		"--json", listFields)
	if err != nil {
		return nil, err
	}
	return parsePRs(out)
}

// listFields are what BranchPR asks gh pr list for.
const listFields = "number,state,isDraft,isCrossRepository,statusCheckRollup,reviewDecision"

// parsePRs reads `gh pr list --json` of listFields, newest first, and picks the newest from the repository
// itself over one from a fork of the same name.
func parsePRs(data []byte) (*PR, error) {
	var prs []struct {
		Number            int         `json:"number"`
		State             string      `json:"state"`
		IsDraft           bool        `json:"isDraft"`
		IsCrossRepository bool        `json:"isCrossRepository"`
		StatusCheckRollup []rollupRun `json:"statusCheckRollup"`
		ReviewDecision    string      `json:"reviewDecision"`
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
	return &PR{Number: pr.Number, State: state, Checks: summarize(pr.StatusCheckRollup), Review: Review(strings.ToLower(pr.ReviewDecision))}, nil
}

// rollupRun is one check of a PR's statusCheckRollup, as gh gives it: a check run, with a status and,
// once completed, a conclusion, or a commit status context, with a state.
type rollupRun struct {
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}

// summarize is what a PR's checks come to: failing when any failed, else pending when any has not
// finished, else passing; none when it has none. Neutral, skipped and stale count as passing, as GitHub
// does not hold a PR back on them.
func summarize(runs []rollupRun) Checks {
	if len(runs) == 0 {
		return ChecksNone
	}
	sum := ChecksPassing
	for _, r := range runs {
		result := r.Conclusion + r.State
		if r.Status != "" && r.Status != "COMPLETED" {
			result = r.Status
		}
		switch result {
		case "FAILURE", "ERROR", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE":
			return ChecksFailing
		case "PENDING", "QUEUED", "IN_PROGRESS", "EXPECTED", "WAITING", "REQUESTED":
			sum = ChecksPending
		}
	}
	return sum
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
