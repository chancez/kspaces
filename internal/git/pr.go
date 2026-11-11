package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// gitDo runs git in the repository at root, its output as the error when it fails. A fetch that needs a
// password fails rather than waiting at a prompt nobody sees.
func gitDo(root string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// TrackRemoteBranch fetches branch from remote and creates the local branch of that name on it, tracking
// it. The refspec is explicit, as a clone of a single branch fetches nothing else into remote/branch.
func TrackRemoteBranch(root, remote, branch string) error {
	upstream := remote + "/" + branch
	if err := gitDo(root, "fetch", remote, "+refs/heads/"+branch+":refs/remotes/"+upstream); err != nil {
		return err
	}
	return gitDo(root, "branch", "--track", branch, upstream)
}

// FetchPullHead fetches PR number's head from remote, a GitHub repository's pull/N/head, into the new
// local branch branch, and has it pull from there, as `gh pr checkout` sets up a PR from a fork it has no
// remote for.
func FetchPullHead(root, remote string, number int, branch string) error {
	ref := fmt.Sprintf("refs/pull/%d/head", number)
	if err := gitDo(root, "fetch", remote, ref+":refs/heads/"+branch); err != nil {
		return err
	}
	if err := gitDo(root, "config", "branch."+branch+".remote", remote); err != nil {
		return err
	}
	return gitDo(root, "config", "branch."+branch+".merge", ref)
}

// BranchMerge is the ref the local branch pulls, its branch.NAME.merge, empty when it has none.
func BranchMerge(root, branch string) string {
	out, err := exec.Command("git", "-C", root, "config", "--get", "branch."+branch+".merge").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Remotes are the URLs of the repository's remotes, by name.
func Remotes(root string) map[string]string {
	ans := map[string]string{}
	out, err := exec.Command("git", "-C", root, "config", "--get-regexp", `^remote\..*\.url$`).Output()
	if err != nil {
		return ans
	}
	for line := range strings.Lines(string(out)) {
		key, url, ok := strings.Cut(strings.TrimSpace(line), " ")
		if name, isURL := strings.CutSuffix(strings.TrimPrefix(key, "remote."), ".url"); ok && isURL {
			ans[name] = url
		}
	}
	return ans
}
