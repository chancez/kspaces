// Package gittest builds throwaway git repositories for tests, kept away from the repository the tests
// run in and from the user's git config.
package gittest

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Repo is a git repository in a directory of the test's.
type Repo struct {
	t   testing.TB
	Dir string
	// commits makes commit timestamps deterministic and increasing.
	commits int
}

// Epoch is the timestamp of the first commit. Each later commit is one second after the previous.
var Epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Isolate keeps git, run by the test or by the code under test, away from the user's repositories and
// settings for the rest of the test. Variables such as GIT_DIR, set when tests run from git rebase --exec
// or a hook, would point git at another repository, so every GIT_ variable is unset, and the global and
// system config are ignored.
func Isolate(t testing.TB) {
	t.Helper()
	for _, kv := range os.Environ() {
		if k, v, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "GIT_") {
			// Setenv restores the variable after the test; Unsetenv then removes it.
			t.Setenv(k, v)
			os.Unsetenv(k)
		}
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// New creates an empty repository with main as its initial branch in a temp directory, symlinks resolved
// so Dir matches what git reports on macOS. It isolates the test's git, as Isolate does.
func New(t testing.TB) *Repo {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return At(t, dir)
}

// At creates an empty repository with main as its initial branch in dir, made if missing, for a test that
// needs it at a particular path. It isolates the test's git, as Isolate does.
func At(t testing.TB, dir string) *Repo {
	t.Helper()
	Isolate(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := &Repo{t: t, Dir: dir}
	r.Git("init", "-q", "-b", "main")
	return r
}

// Bare creates an empty bare repository in a temp directory, to push to and fetch from as a remote. It
// isolates the test's git, as Isolate does.
func Bare(t testing.TB) *Repo {
	t.Helper()
	Isolate(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := &Repo{t: t, Dir: dir}
	r.Git("init", "-q", "--bare", "-b", "main")
	return r
}

// Git runs git in the repository and returns its trimmed stdout, failing the test with git's stderr if it
// fails.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()
	return r.gitEnv(nil, args...)
}

func (r *Repo) gitEnv(env []string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	// The identity is only for this command, so the code under test sees the environment it would.
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			stderr = exitErr.Stderr
		}
		r.t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), r.Dir, err, stderr)
	}
	return strings.TrimSpace(string(out))
}

// Write writes files, given as path and content pairs, relative to the repository root.
func (r *Repo) Write(pathContent ...string) {
	r.t.Helper()
	if len(pathContent)%2 != 0 {
		r.t.Fatal("Write needs path and content pairs")
	}
	for i := 0; i < len(pathContent); i += 2 {
		path := filepath.Join(r.Dir, pathContent[i])
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(pathContent[i+1]), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
}

// Commit stages everything and commits it, returning the new sha.
func (r *Repo) Commit(message string) string {
	r.t.Helper()
	date := Epoch.Add(time.Duration(r.commits) * time.Second).Format(time.RFC3339)
	r.commits++
	r.Git("add", "-A")
	r.gitEnv([]string{"GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date},
		"commit", "-q", "--allow-empty", "--no-verify", "-m", message)
	return r.Git("rev-parse", "HEAD")
}

// Worktree adds a linked worktree on a new branch at path, relative to the repository root or absolute,
// and returns it as a Repo sharing this one's commit dates.
func (r *Repo) Worktree(branch, path string) *Repo {
	r.t.Helper()
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.Dir, path)
	}
	r.Git("worktree", "add", "-q", "-b", branch, path)
	return &Repo{t: r.t, Dir: path, commits: r.commits}
}

// Clone clones the repository into dir and returns the clone.
func (r *Repo) Clone(dir string) *Repo {
	r.t.Helper()
	r.Git("clone", "-q", r.Dir, dir)
	return &Repo{t: r.t, Dir: dir, commits: r.commits}
}
