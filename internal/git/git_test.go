package git

import (
	"reflect"
	"testing"
)

func TestParseWorktrees(t *testing.T) {
	porcelain := `worktree /src/gr
HEAD 1111111111111111111111111111111111111111
branch refs/heads/main

worktree /src/gr/.worktrees/dev
HEAD 2222222222222222222222222222222222222222
branch refs/heads/pr/chancez/dev

worktree /src/gr/.worktrees/gone
HEAD 3333333333333333333333333333333333333333
branch refs/heads/old
prunable gitdir file points to non-existent location

worktree /src/gr/.worktrees/bisect
HEAD 4444444444444444444444444444444444444444
detached
`
	got := ParseWorktrees(porcelain)
	want := []Worktree{
		{Path: "/src/gr", Branch: "main"},
		{Path: "/src/gr/.worktrees/dev", Branch: "pr/chancez/dev"},
		{Path: "/src/gr/.worktrees/bisect", Branch: ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

// A bare repository has no files to open, so its first checked out worktree comes first.
func TestParseWorktreesBare(t *testing.T) {
	porcelain := `worktree /src/gr.git
bare

worktree /src/gr/main
HEAD 1111111111111111111111111111111111111111
branch refs/heads/main
`
	got := ParseWorktrees(porcelain)
	want := []Worktree{{Path: "/src/gr/main", Branch: "main"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}
