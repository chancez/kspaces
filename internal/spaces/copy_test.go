package spaces

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// A copy's first tab is a shell the launch wrapper puts on the host, when the provider says it does, and
// else kitten ssh there, in the copy's directory, the window here in the home directory; each with the
// launch variables that say where it belongs.
func TestRemoteTab(t *testing.T) {
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Launch: []string{"launch"}, Hosts: map[string]config.HostSettings{"white": {SSH: "me@white.lan"}}}
	wt := Worktree{Path: "/home/me/gr/.worktrees/dev", Session: "white:gr@dev", Host: "white"}
	vars := []string{"--add-to-session=white:gr@dev", "--var=kspaces_space=gr", "--var=kspaces_worktree=dev", "--var=kspaces_host=white", "--var=kspaces_dir=/home/me/gr/.worktrees/dev"}
	start := append([]string{"launch", "--type=tab", "--cwd=" + home}, vars...)
	got := [][]string{}
	for _, caps := range []Capabilities{{CapHosts: true}, nil} {
		tab, err := remoteTab(cfg, caps, wt)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, tab)
	}
	def, err := remoteTab(cfg, nil, Worktree{Session: "white:default", Host: "white"})
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, def)
	want := [][]string{
		start,
		append(append([]string{}, start...), "kitten", "ssh", "--kitten", "cwd=/home/me/gr/.worktrees/dev", "me@white.lan"),
		{"launch", "--type=tab", "--cwd=" + home, "--add-to-session=white:default", "--var=kspaces_space=default", "--var=kspaces_host=white", "kitten", "ssh", "me@white.lan"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tabs\n%q\nwant\n%q", got, want)
	}
}

// A copy is found by its session's name on its host, its space's name for its main worktree, and HOST:
// alone is the host's default; a host the config does not name is an error.
func TestFindCopy(t *testing.T) {
	data := HostData{Config: &config.Config{}, Spaces: []sharedSpace{{Name: "gr", Root: "/home/me/gr", Worktrees: []Worktree{
		{Path: "/home/me/gr", Branch: "main", Session: "gr"},
		{Path: "/home/me/gr/.worktrees/dev", Branch: "dev", Session: "gr@dev"},
	}}}}
	UseRemotes(func(name string, h config.HostSettings) Host { return &fakeHost{data: data} })
	t.Cleanup(func() { UseRemotes(nil) })
	cfg := config.Config{Hosts: map[string]config.HostSettings{"white": {}}}
	var got []Worktree
	for _, session := range []string{"gr@dev", "gr", ""} {
		wt, err := FindCopy(cfg, "white", session)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, wt)
	}
	want := []Worktree{
		{Path: "/home/me/gr/.worktrees/dev", Branch: "dev", Session: "white:gr@dev", Host: "white"},
		{Path: "/home/me/gr", Branch: "main", Session: "white:gr", Host: "white"},
		{Session: "white:default", Host: "white"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("copies\n%+v\nwant\n%+v", got, want)
	}
	if _, err := FindCopy(cfg, "white", "gr@gone"); err == nil || err.Error() != "white has no worktree whose session is gr@gone" {
		t.Errorf("a worktree white has not: %v", err)
	}
	if _, err := FindCopy(cfg, "nas", "gr"); !errors.Is(err, ErrNoHost) {
		t.Errorf("a host not named: %v", err)
	}
}

// Another host's copies open here are not sessions in no space: the hosts tab lists them under their host.
func TestCopiesNotOthers(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cfg := config.Config{Hosts: map[string]config.HostSettings{"white": {}}}
	sh := Shared{Config: &cfg, Saved: []string{"white:gr", "notes"}, Windows: []kitty.OSWindow{{Tabs: []kitty.Tab{
		{ID: 1, Windows: []kitty.Window{{ID: 10, SessionName: "white:gr@dev"}}},
		{ID: 2, Windows: []kitty.Window{{ID: 20, SessionName: "nas:scratch"}}},
	}}}}
	var names []string
	for _, o := range sh.View(10).Others {
		names = append(names, o.Name)
	}
	if want := []string{"nas:scratch", "notes"}; !reflect.DeepEqual(names, want) {
		t.Errorf("others %q, want %q", names, want)
	}
}
