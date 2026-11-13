package gh

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chancez/kspaces/internal/gittest"
)

// fakeGH puts a gh first on PATH that runs script, after recording the directory it ran in and its
// arguments, a line each. It returns them as the last call had them, nil when gh did not run.
func fakeGH(t *testing.T, script string) func() []string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "args")
	script = "#!/bin/sh\n{ pwd -P; for a; do printf '%s\\n' \"$a\"; done; } > " + log + "\n" + script
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() []string {
		data, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	}
}

// realDir is a temporary directory as pwd -P says it.
func realDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// gh pr view runs in the directory given, and what it says of a PR from a fork comes back flat.
func TestViewPR(t *testing.T) {
	dir := realDir(t)
	args := fakeGH(t, `echo '{"author":{"login":"forker"},"headRefName":"main","headRepositoryOwner":{"id":"U_2","login":"forker"},"isCrossRepository":true,"isDraft":true,"number":7,"title":"Fix from a fork","url":"https://github.com/o/r/pull/7"}'`+"\n")
	got, err := ViewPR(dir, "https://github.com/o/r/pull/7")
	if err != nil {
		t.Fatal(err)
	}
	if want := (PRView{Number: 7, Title: "Fix from a fork", Author: "forker", Draft: true, HeadRef: "main", Owner: "forker", Cross: true, URL: "https://github.com/o/r/pull/7"}); got != want {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if got, want := args(), []string{dir, "pr", "view", "https://github.com/o/r/pull/7", "--json", viewFields}; !reflect.DeepEqual(got, want) {
		t.Errorf("gh ran as %q\nwant %q", got, want)
	}
}

// gh pr list lists the open PRs, newest first as gh has them, each as gh pr view says it; its error is
// gh's message.
func TestOpenPRs(t *testing.T) {
	dir := realDir(t)
	args := fakeGH(t, `echo '[{"author":{"login":"me"},"headRefName":"feat","headRepositoryOwner":{"login":"o"},"isCrossRepository":false,"isDraft":false,"number":9,"title":"Add feat","url":"https://github.com/o/r/pull/9"},`+
		`{"author":{"login":"forker"},"headRefName":"main","headRepositoryOwner":{"login":"forker"},"isCrossRepository":true,"isDraft":true,"number":7,"title":"Fix","url":"https://github.com/o/r/pull/7"}]'`+"\n")
	got, err := OpenPRs(dir)
	want := []PRView{
		{Number: 9, Title: "Add feat", Author: "me", HeadRef: "feat", Owner: "o", URL: "https://github.com/o/r/pull/9"},
		{Number: 7, Title: "Fix", Author: "forker", Draft: true, HeadRef: "main", Owner: "forker", Cross: true, URL: "https://github.com/o/r/pull/7"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, %v\nwant %+v", got, err, want)
	}
	if got, want := args(), []string{dir, "pr", "list", "--state", "open", "--limit", "100", "--json", viewFields}; !reflect.DeepEqual(got, want) {
		t.Errorf("gh ran as %q\nwant %q", got, want)
	}
	fakeGH(t, "echo 'no git remotes found' >&2\nexit 1\n")
	if _, err := OpenPRs(dir); err == nil || err.Error() != "no git remotes found" {
		t.Errorf("gh failing: %v", err)
	}
}

// gh's own message comes back, and so does gh being missing, or saying nothing as it fails, or what it
// printed not being JSON.
func TestViewPRErrors(t *testing.T) {
	dir := realDir(t)
	var got []string
	view := func() {
		_, err := ViewPR(dir, "404")
		got = append(got, err.Error())
	}
	fakeGH(t, "echo 'GraphQL: Could not resolve to a PullRequest with the number of 404. (repository.pullRequest)' >&2\nexit 1\n")
	view()
	fakeGH(t, "exit 4\n")
	view()
	fakeGH(t, "echo nope\n")
	view()
	t.Setenv("PATH", t.TempDir())
	view()
	want := []string{
		"GraphQL: Could not resolve to a PullRequest with the number of 404. (repository.pullRequest)",
		"gh pr view: exit status 4",
		"parsing gh pr view: invalid character 'o' in literal null (expecting 'u')",
		"gh is not installed, and looking up a PR needs it",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

// gh pr list runs in the repository for the branch, and the repository's default branch, or no branch,
// is not asked about.
func TestBranchPR(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	args := fakeGH(t, `echo '[{"number":12,"state":"OPEN","isDraft":true,"isCrossRepository":false,"reviewDecision":"REVIEW_REQUIRED","statusCheckRollup":[{"__typename":"CheckRun","conclusion":"SUCCESS","name":"test","status":"COMPLETED","workflowName":"CI"}]}]'`+"\n")
	pr, err := BranchPR(r.Dir, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if want := (&PR{Number: 12, State: "draft", Checks: ChecksPassing, Review: ReviewRequired}); !reflect.DeepEqual(pr, want) {
		t.Errorf("got %+v, want %+v", pr, want)
	}
	if got, want := args(), []string{r.Dir, "pr", "list", "--state", "all", "--head", "dev", "--limit", "10", "--json", "number,state,isDraft,isCrossRepository,statusCheckRollup,reviewDecision"}; !reflect.DeepEqual(got, want) {
		t.Errorf("gh ran as %q\nwant %q", got, want)
	}

	args = fakeGH(t, "exit 1\n")
	for _, branch := range []string{"main", ""} {
		if pr, err := BranchPR(r.Dir, branch); pr != nil || err != nil {
			t.Errorf("branch %q: got %+v, %v", branch, pr, err)
		}
	}
	if got := args(); got != nil {
		t.Errorf("gh ran as %q for the default branch or none", got)
	}
}

func TestParsePRs(t *testing.T) {
	for name, tc := range map[string]struct {
		in   string
		want *PR
	}{
		"open":   {`[{"number":12,"state":"OPEN","isDraft":false,"isCrossRepository":false}]`, &PR{Number: 12, State: "open"}},
		"draft":  {`[{"number":13,"state":"OPEN","isDraft":true,"isCrossRepository":false}]`, &PR{Number: 13, State: "draft"}},
		"merged": {`[{"number":14,"state":"MERGED","isDraft":false,"isCrossRepository":false}]`, &PR{Number: 14, State: "merged"}},
		"closed": {`[{"number":15,"state":"CLOSED","isDraft":false,"isCrossRepository":false}]`, &PR{Number: 15, State: "closed"}},
		"none":   {`[]`, nil},
		// The newest from the repository itself, over a fork's of the same name.
		"a fork's first": {`[{"number":20,"state":"OPEN","isCrossRepository":true},{"number":16,"state":"MERGED","isCrossRepository":false},{"number":9,"state":"CLOSED","isCrossRepository":false}]`, &PR{Number: 16, State: "merged"}},
		"only a fork's":  {`[{"number":20,"state":"OPEN","isCrossRepository":true}]`, &PR{Number: 20, State: "open"}},
		// gh's empty rollup and review decision, as for a PR with no CI and no review rules.
		"no checks or review": {`[{"number":21,"state":"OPEN","isDraft":false,"isCrossRepository":false,"reviewDecision":"","statusCheckRollup":[]}]`, &PR{Number: 21, State: "open"}},
		"approved":            {`[{"number":22,"state":"OPEN","isDraft":false,"isCrossRepository":false,"reviewDecision":"APPROVED","statusCheckRollup":[]}]`, &PR{Number: 22, State: "open", Review: ReviewApproved}},
		"changes requested":   {`[{"number":23,"state":"OPEN","isDraft":false,"isCrossRepository":false,"reviewDecision":"CHANGES_REQUESTED","statusCheckRollup":[]}]`, &PR{Number: 23, State: "open", Review: ReviewChangesRequested}},
		"review required":     {`[{"number":24,"state":"OPEN","isDraft":false,"isCrossRepository":false,"reviewDecision":"REVIEW_REQUIRED","statusCheckRollup":[]}]`, &PR{Number: 24, State: "open", Review: ReviewRequired}},
		// The checks of the PR picked, not of the fork's PR before it.
		"a fork's checks": {`[{"number":25,"state":"OPEN","isCrossRepository":true,"statusCheckRollup":[` + checkRun("COMPLETED", "FAILURE") + `]},{"number":26,"state":"OPEN","isCrossRepository":false,"statusCheckRollup":[` + checkRun("COMPLETED", "SUCCESS") + `]}]`, &PR{Number: 26, State: "open", Checks: ChecksPassing}},
	} {
		got, err := parsePRs([]byte(tc.in))
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %+v, %v, want %+v", name, got, err, tc.want)
		}
	}
	if _, err := parsePRs([]byte("not json")); err == nil || err.Error() != "parsing gh pr list: invalid character 'o' in literal null (expecting 'u')" {
		t.Errorf("output that is not JSON: %v", err)
	}
}

// checkRun is a check run as gh pr list gives it in statusCheckRollup, with conclusion empty until it is
// completed.
func checkRun(status, conclusion string) string {
	return `{"__typename":"CheckRun","completedAt":"2026-11-13T10:00:00Z","conclusion":"` + conclusion + `","detailsUrl":"https://github.com/o/r/actions/runs/1/job/2","name":"test","startedAt":"2026-11-13T09:58:00Z","status":"` + status + `","workflowName":"CI"}`
}

// statusContext is a commit status as gh pr list gives it in statusCheckRollup.
func statusContext(state string) string {
	return `{"__typename":"StatusContext","context":"ci/circleci","startedAt":"2026-11-13T09:58:00Z","state":"` + state + `","targetUrl":"https://circleci.com/gh/o/r/1"}`
}

// A PR's checks are failing when any failed, else pending when any has not finished, else passing, and
// none when it has none: check runs by their status and conclusion, commit statuses by their state.
func TestParsePRsChecks(t *testing.T) {
	for name, tc := range map[string]struct {
		runs []string
		want Checks
	}{
		"none":                           {nil, ChecksNone},
		"a run passed":                   {[]string{checkRun("COMPLETED", "SUCCESS")}, ChecksPassing},
		"a status passed":                {[]string{statusContext("SUCCESS")}, ChecksPassing},
		"neutral and skipped pass":       {[]string{checkRun("COMPLETED", "NEUTRAL"), checkRun("COMPLETED", "SKIPPED"), statusContext("SUCCESS")}, ChecksPassing},
		"a run failed":                   {[]string{checkRun("COMPLETED", "SUCCESS"), checkRun("COMPLETED", "FAILURE")}, ChecksFailing},
		"a run cancelled":                {[]string{checkRun("COMPLETED", "CANCELLED")}, ChecksFailing},
		"a run timed out":                {[]string{checkRun("COMPLETED", "TIMED_OUT")}, ChecksFailing},
		"a run needs action":             {[]string{checkRun("COMPLETED", "ACTION_REQUIRED")}, ChecksFailing},
		"a status errored":               {[]string{statusContext("ERROR")}, ChecksFailing},
		"a status failed":                {[]string{statusContext("FAILURE")}, ChecksFailing},
		"a run queued":                   {[]string{checkRun("COMPLETED", "SUCCESS"), checkRun("QUEUED", "")}, ChecksPending},
		"a run in progress":              {[]string{checkRun("IN_PROGRESS", "")}, ChecksPending},
		"a status pending":               {[]string{checkRun("COMPLETED", "SUCCESS"), statusContext("PENDING")}, ChecksPending},
		"a status expected":              {[]string{statusContext("EXPECTED")}, ChecksPending},
		"failing over pending":           {[]string{checkRun("IN_PROGRESS", ""), statusContext("PENDING"), checkRun("COMPLETED", "FAILURE"), checkRun("COMPLETED", "SUCCESS")}, ChecksFailing},
		"a failed status over a pending": {[]string{checkRun("QUEUED", ""), statusContext("ERROR")}, ChecksFailing},
	} {
		in := `[{"number":30,"state":"OPEN","isDraft":false,"isCrossRepository":false,"reviewDecision":"","statusCheckRollup":[` + strings.Join(tc.runs, ",") + `]}]`
		got, err := parsePRs([]byte(in))
		if want := (&PR{Number: 30, State: "open", Checks: tc.want}); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %+v, %v, want %+v", name, got, err, want)
		}
	}
}
