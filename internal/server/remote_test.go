package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSSH puts an ssh on PATH that runs script, with what it was run with in $ARGS_FILE.
func fakeSSH(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	args := filepath.Join(dir, "args")
	body := "#!/bin/sh\necho \"$@\" > " + args + "\n" + script + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return args
}

// Another host is reached by ssh running kspaces server proxy there, batch mode, and what ssh says when it
// cannot connect is the error.
func TestRemoteSSH(t *testing.T) {
	args := fakeSSH(t, "echo 'ssh: connect to host white port 22: Connection refused' >&2; exit 255")
	_, err := NewRemote("me@white", "~/bin/kspaces", "v1").Load(true)
	if err == nil || !strings.Contains(err.Error(), "ssh: connect to host white port 22: Connection refused") {
		t.Errorf("Load() = %v, want ssh's error", err)
	}
	got, _ := os.ReadFile(args)
	if want := "-T -o BatchMode=yes -o ConnectTimeout=10 me@white ~/bin/kspaces server proxy\n"; string(got) != want {
		t.Errorf("ssh ran with %q, want %q", got, want)
	}
}

// A proxy on the other host is the server there: the fake ssh runs a server of its own on its stdin and
// stdout, as kspaces server proxy connects them to one.
func TestRemoteLoads(t *testing.T) {
	if os.Getenv("KSPACES_TEST_PROXY") == "1" {
		New(&fakeHost{data: grData}, "v1", "white").ServeConn(t.Context(), stdio{})
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	fakeSSH(t, "KSPACES_TEST_PROXY=1 exec "+exe+" -test.run '^TestRemoteLoads$'")
	c := NewRemote("white", "kspaces", "v1")
	defer c.Close()
	hello, err := c.Hello()
	if err != nil {
		t.Fatal(err)
	}
	if want := (Hello{Protocol: Protocol, Version: "v1", Host: "white"}); hello != want {
		t.Errorf("hello %+v, want %+v", hello, want)
	}
	if got, err := c.Load(false); err != nil || got.Config == nil || got.Config.SessionsDir != "/sessions" {
		t.Errorf("Load() = %+v, %v", got, err)
	}
}

// stdio is the process's stdin and stdout as one connection.
type stdio struct{}

func (stdio) Read(b []byte) (int, error)  { return os.Stdin.Read(b) }
func (stdio) Write(b []byte) (int, error) { return os.Stdout.Write(b) }
func (stdio) Close() error                { return os.Stdin.Close() }
