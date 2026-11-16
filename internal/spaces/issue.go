package spaces

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/git"
)

// issueURL is an issue's page, on github.com or a GitHub Enterprise host: HOST/OWNER/REPO/issues/N, and
// maybe a part of it such as #issuecomment-1.
var issueURL = regexp.MustCompile(`^https?://[^/\s]+/[^/\s]+/[^/\s]+/issues/([0-9]+)(?:[/?#]\S*)?$`)

// ParseIssueRef says whether s names an issue: N, #N or an issue's URL.
func ParseIssueRef(s string) (Ref, bool) { return parseRef(issueURL, s) }

// IsIssueURL says whether s is an issue's page.
func IsIssueURL(s string) bool { return issueURL.MatchString(strings.TrimSpace(s)) }

// ResolveIssue looks ref up with gh, in the space's root.
func ResolveIssue(space Space, ref Ref) (gh.IssueView, error) {
	return gh.ViewIssue(space.Root, ref.Arg)
}

// issueSlug is how long the words of an issue's title in its branch's name can be, so the name stays
// short enough to read in the sidebar.
const issueSlug = 40

// IssueBranch is the branch a worktree for issue number, titled title, is created on: the configured
// prefix, then the number and the title's words, lowercased, letters and digits only, joined by -, as
// many whole words as fit in issueSlug: pr/me/12-crash-on-empty-input.
func IssueBranch(cfg config.Config, number int, title string) string {
	var words []string
	n := 0
	for _, w := range strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		if n+len(w) > issueSlug {
			break
		}
		words = append(words, w)
		n += len(w) + 1
	}
	return BranchStart(cfg) + strings.Join(append([]string{strconv.Itoa(number)}, words...), "-")
}

// CreateIssueWorktree adds a worktree at dir to the space on branch, as CreateWorktree does, and records
// issue, by number, as the one it is for, so {issue} is known in it from then on, see WorktreeIssue.
func CreateIssueWorktree(space Space, issue int, branch, dir string) (Worktree, error) {
	wt, err := CreateWorktree(space, branch, dir)
	if err != nil {
		return Worktree{}, err
	}
	if err := git.SetBranchIssue(space.Root, wt.Branch, issue); err != nil {
		return Worktree{}, fmt.Errorf("recording issue #%d for %s: %w", issue, wt.Branch, err)
	}
	return wt, nil
}
