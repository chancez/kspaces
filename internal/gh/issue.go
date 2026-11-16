package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// IssueView is what ViewIssue and OpenIssues say of an issue, as much as choosing one needs.
type IssueView struct {
	Number int
	Title  string
	// Who opened it, by login.
	Author string
	// The issue's page.
	URL string
}

// issueFields are what ViewIssue and OpenIssues ask gh for, IssueView's.
const issueFields = "number,title,author,url"

// issueJSON is an issue as gh's --json of issueFields has it.
type issueJSON struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	URL string `json:"url"`
}

func (i issueJSON) view() IssueView {
	return IssueView{Number: i.Number, Title: i.Title, Author: i.Author.Login, URL: i.URL}
}

// ViewIssue looks up the issue issue, a number or a URL, with gh run in dir, as ViewPR looks up a PR.
func ViewIssue(dir, issue string) (IssueView, error) {
	out, err := run(context.Background(), dir, "looking up an issue", "issue", "view", issue, "--json", issueFields)
	if err != nil {
		return IssueView{}, err
	}
	var got issueJSON
	if err := json.Unmarshal(out, &got); err != nil {
		return IssueView{}, fmt.Errorf("parsing gh issue view: %w", err)
	}
	return got.view(), nil
}

// OpenIssues are the open issues of the GitHub repository dir's remotes point to, newest first, as many
// as openLimit, with gh run in dir, as OpenPRs lists the open PRs.
func OpenIssues(dir string) ([]IssueView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), branchTimeout)
	defer cancel()
	out, err := run(ctx, dir, "listing issues", "issue", "list", "--state", "open", "--limit", strconv.Itoa(openLimit), "--json", issueFields)
	if err != nil {
		return nil, err
	}
	var got []issueJSON
	if err := json.Unmarshal(out, &got); err != nil {
		return nil, fmt.Errorf("parsing gh issue list: %w", err)
	}
	views := make([]IssueView, len(got))
	for i, v := range got {
		views[i] = v.view()
	}
	return views, nil
}
