package gh

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
	args := fakeGH(t, `echo '{"headRefName":"main","headRepositoryOwner":{"id":"U_2","login":"forker"},"isCrossRepository":true,"number":7,"title":"Fix from a fork","url":"https://github.com/o/r/pull/7"}'`+"\n")
	got, err := ViewPR(dir, "https://github.com/o/r/pull/7")
	if err != nil {
		t.Fatal(err)
	}
	if want := (PRView{Number: 7, Title: "Fix from a fork", HeadRef: "main", Owner: "forker", Cross: true, URL: "https://github.com/o/r/pull/7"}); got != want {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if got, want := args(), []string{dir, "pr", "view", "https://github.com/o/r/pull/7", "--json", "number,headRefName,headRepositoryOwner,isCrossRepository,title,url"}; !reflect.DeepEqual(got, want) {
		t.Errorf("gh ran as %q\nwant %q", got, want)
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
