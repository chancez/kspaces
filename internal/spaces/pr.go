package spaces

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/git"
)

// PRRef is a PR as typed: its number, or its URL.
type PRRef struct {
	// What gh pr view is given: the number, or the URL as typed.
	Arg    string
	Number int
}

// prURL is a PR's page, on github.com or a GitHub Enterprise host: HOST/OWNER/REPO/pull/N, and maybe a
// tab of it such as /files.
var prURL = regexp.MustCompile(`^https?://[^/\s]+/[^/\s]+/[^/\s]+/pull/([0-9]+)(?:[/?#]\S*)?$`)

// ParsePRRef says whether s names a PR: N, #N or a PR's URL.
func ParsePRRef(s string) (PRRef, bool) {
	s = strings.TrimSpace(s)
	if m := prURL.FindStringSubmatch(s); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return PRRef{Arg: s, Number: n}, true
		}
		return PRRef{}, false
	}
	digits := strings.TrimPrefix(s, "#")
	if digits == "" || strings.Trim(digits, "0123456789") != "" {
		return PRRef{}, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n <= 0 {
		return PRRef{}, false
	}
	return PRRef{Arg: strconv.Itoa(n), Number: n}, true
}

// IsPRURL says whether s is a PR's page.
func IsPRURL(s string) bool { return prURL.MatchString(strings.TrimSpace(s)) }

// PR is a pull request to check out in a new worktree, and how.
type PR struct {
	Number int
	Title  string
	// The PR's branch in the repository it is from, and that repository's owner.
	HeadRef, Owner string
	// From another repository than the one it is to, such as a fork, so fetched from pull/N/head.
	Cross bool
	// Where it is fetched from: the space's remote for the repository the PR is to, else origin.
	Remote string
	// The local branch the worktree checks out.
	Branch string
	// Whether Branch is there already, and is checked out as it is.
	Exists bool
}

// ResolvePR looks ref up with gh, in the space's root, and works out how to check it out, see CheckoutPR.
func ResolvePR(space Space, ref PRRef) (PR, error) {
	got, err := gh.ViewPR(space.Root, ref.Arg)
	if err != nil {
		return PR{}, err
	}
	return CheckoutPR(space, got)
}

// CheckoutPR works out how to check out view, a PR as gh says it, in the space. A PR from the repository
// itself is its branch, tracking the remote's. A PR from a fork is fetched into a local branch named after
// the fork's, or pr-N-BRANCH when the space has a branch of that name that is not this PR's, as a fork's
// main would clash with the space's own.
func CheckoutPR(space Space, view gh.PRView) (PR, error) {
	pr := PR{
		Number:  view.Number,
		Title:   view.Title,
		HeadRef: view.HeadRef,
		Owner:   view.Owner,
		Cross:   view.Cross,
		Remote:  prRemote(git.Remotes(space.Root), view.URL),
	}
	if !pr.Cross {
		pr.Branch, pr.Exists = pr.HeadRef, space.HasBranch(pr.HeadRef)
		return pr, nil
	}
	pull := fmt.Sprintf("refs/pull/%d/head", pr.Number)
	names := []string{pr.HeadRef, fmt.Sprintf("pr-%d-%s", pr.Number, pr.HeadRef)}
	for _, name := range names {
		switch {
		case !space.HasBranch(name):
			pr.Branch = name
			return pr, nil
		case git.BranchMerge(space.Root, name) == pull:
			// Set up for this PR before, by an earlier worktree.
			pr.Branch, pr.Exists = name, true
			return pr, nil
		}
	}
	return PR{}, fmt.Errorf("the branches %s and %s are taken, so PR #%d needs a branch of its own", names[0], names[1], pr.Number)
}

// prRemote is the remote, of remotes by name, of the repository the PR at url is to, origin first when
// several are, and origin when none is, such as for a remote known by a path.
func prRemote(remotes map[string]string, url string) string {
	want := repoOf(url)
	if i := strings.Index(want, "/pull/"); i >= 0 {
		want = want[:i]
	}
	names := make([]string, 0, len(remotes))
	for name := range remotes {
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b string) int {
		if (a == "origin") != (b == "origin") {
			if a == "origin" {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	for _, name := range names {
		if want != "" && repoOf(remotes[name]) == want {
			return name
		}
	}
	return "origin"
}

// repoOf is a repository's URL as HOST/OWNER/REPO, lowercased, whether written https://HOST/OWNER/REPO,
// ssh://USER@HOST:PORT/OWNER/REPO or USER@HOST:OWNER/REPO, with or without .git.
func repoOf(url string) string {
	u := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(url), "/"), ".git")
	if _, rest, ok := strings.Cut(u, "://"); ok {
		u = rest
	} else if host, path, ok := strings.Cut(u, ":"); ok && !strings.Contains(host, "/") {
		u = host + "/" + path
	} else {
		return ""
	}
	host, path, _ := strings.Cut(u, "/")
	if _, h, ok := strings.Cut(host, "@"); ok {
		host = h
	}
	host, _, _ = strings.Cut(host, ":")
	return strings.ToLower(host + "/" + path)
}

// CreatePRWorktree adds a worktree at dir to the space, on the PR's branch, fetching it into a new local
// branch first unless the space has it, as after a worktree that git refused.
func CreatePRWorktree(space Space, pr PR, dir string) (Worktree, error) {
	if !space.HasBranch(pr.Branch) {
		var err error
		if pr.Cross {
			err = git.FetchPullHead(space.Root, pr.Remote, pr.Number, pr.Branch)
		} else {
			err = git.TrackRemoteBranch(space.Root, pr.Remote, pr.Branch)
		}
		if err != nil {
			return Worktree{}, err
		}
	}
	return CreateWorktree(space, pr.Branch, dir)
}
