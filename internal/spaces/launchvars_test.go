package spaces

import (
	"reflect"
	"testing"
)

// Every window kspaces opens in a session says the space and worktree its session is, for a launch
// wrapper, which cannot see the session, and a window of another host's copy says the host.
func TestLaunchVars(t *testing.T) {
	got := map[string][]string{}
	for _, session := range []string{"gr", "gr@fix", "default", "hubble-timescape@version-skew", "gr@a dir", "my space", ".hidden", "gr@", "white:gr@dev", "white:default"} {
		got[session] = inSession(session)
	}
	want := map[string][]string{
		"gr":                            {"--add-to-session=gr", "--var=kspaces_space=gr"},
		"gr@fix":                        {"--add-to-session=gr@fix", "--var=kspaces_space=gr", "--var=kspaces_worktree=fix"},
		"default":                       {"--add-to-session=default", "--var=kspaces_space=default"},
		"hubble-timescape@version-skew": {"--add-to-session=hubble-timescape@version-skew", "--var=kspaces_space=hubble-timescape", "--var=kspaces_worktree=version-skew"},
		"gr@a dir":                      {"--add-to-session=gr@a dir", "--var=kspaces_space=gr", "'--var=kspaces_worktree=a dir'"},
		// Whatever the name, it is the provider's to take or refuse.
		"my space":      {"--add-to-session=my space", "'--var=kspaces_space=my space'"},
		".hidden":       {"--add-to-session=.hidden", "--var=kspaces_space=.hidden"},
		"gr@":           {"--add-to-session=gr@", "--var=kspaces_space=gr"},
		"white:gr@dev":  {"--add-to-session=white:gr@dev", "--var=kspaces_space=gr", "--var=kspaces_worktree=dev", "--var=kspaces_host=white"},
		"white:default": {"--add-to-session=white:default", "--var=kspaces_space=default", "--var=kspaces_host=white"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
