package cli

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

// remoteHost is a spaces.Host whose loads are data.
type remoteHost struct {
	spaces.Host
	data spaces.HostData
}

func (h remoteHost) Load(bool) (spaces.HostData, error) { return h.data, nil }

// open completes the hosts the config names, going on with no space to a host's sessions, as its server
// lists them.
func TestOpenCompletions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	spaces.UseRemotes(func(string, config.HostSettings) spaces.Host {
		var data spaces.HostData
		if err := jsonUnmarshal(`{"config": {}, "spaces": [{"name": "homelab", "root": "/home/me/homelab", "worktrees": [
			{"Path": "/home/me/homelab", "Session": "homelab"}, {"Path": "/home/me/homelab/.worktrees/dns", "Session": "homelab@dns"}]}]}`, &data); err != nil {
			t.Fatal(err)
		}
		return remoteHost{data: data}
	})
	t.Cleanup(func() { spaces.UseRemotes(nil) })
	cfg := config.Config{Hosts: map[string]config.HostSettings{"white": {}, "nas": {}}}
	type result struct {
		Completions []string
		Directive   cobra.ShellCompDirective
	}
	got := map[string]result{}
	for _, q := range []string{"", "wh", "white:", "white:homelab@"} {
		c, d := openCompletions(cfg, q)
		got[q] = result{c, d}
	}
	want := map[string]result{
		"":               {[]string{"nas:", "white:"}, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace},
		"wh":             {[]string{"white:"}, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace},
		"white:":         {[]string{"white:", "white:homelab", "white:homelab@dns"}, cobra.ShellCompDirectiveNoFileComp},
		"white:homelab@": {[]string{"white:homelab@dns"}, cobra.ShellCompDirectiveNoFileComp},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("completions\n%+v\nwant\n%+v", got, want)
	}
}

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }
