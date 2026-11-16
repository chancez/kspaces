package gh

import (
	"reflect"
	"testing"
)

// gh issue view and gh issue list run in the directory given, what they say of an issue coming back flat;
// the error is gh's message.
func TestIssues(t *testing.T) {
	dir := realDir(t)
	issue := `{"author":{"login":"me"},"number":12,"title":"Crash on empty input","url":"https://github.com/o/r/issues/12"}`
	type result struct {
		View  IssueView
		Open  []IssueView
		Calls [][]string
	}
	args := fakeGH(t, "echo '"+issue+"'\n")
	view, err := ViewIssue(dir, "https://github.com/o/r/issues/12")
	if err != nil {
		t.Fatal(err)
	}
	calls := [][]string{args()}
	args = fakeGH(t, "echo '["+issue+`,{"author":{"login":"you"},"number":9,"title":"Docs","url":"https://github.com/o/r/issues/9"}]'`+"\n")
	open, err := OpenIssues(dir)
	if err != nil {
		t.Fatal(err)
	}
	calls = append(calls, args())
	got := result{view, open, calls}
	twelve := IssueView{Number: 12, Title: "Crash on empty input", Author: "me", URL: "https://github.com/o/r/issues/12"}
	want := result{twelve, []IssueView{twelve, {Number: 9, Title: "Docs", Author: "you", URL: "https://github.com/o/r/issues/9"}}, [][]string{
		{dir, "issue", "view", "https://github.com/o/r/issues/12", "--json", issueFields},
		{dir, "issue", "list", "--state", "open", "--limit", "100", "--json", issueFields},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	fakeGH(t, "echo 'GraphQL: Could not resolve to an issue or pull request with the number of 404.' >&2\nexit 1\n")
	if _, err := ViewIssue(dir, "404"); err == nil || err.Error() != "GraphQL: Could not resolve to an issue or pull request with the number of 404." {
		t.Errorf("gh failing: %v", err)
	}
}
