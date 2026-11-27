package cli

import (
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

func (a *app) openCommand() *cobra.Command {
	var noLayout bool
	cmd := &cobra.Command{
		Use:   "open [PATH | HOST:[SESSION]]",
		Short: "Switch to the session of the worktree at PATH, the current directory's by default, creating it if needed",
		Long: `Switch to the session of the worktree at PATH, the current directory's by default, creating it if needed.

HOST:SESSION opens another host's copy of the worktree whose session there is SESSION, such as white:gr@dev
or white:homelab, in a kitty session of that name, its windows on that host; HOST: alone is a shell there.
HOST is one of [hosts] in the config.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			opts := spaces.OpenOptions{NoLayout: noLayout}
			if host, session := spaces.SplitCopy(dir); host != "" {
				if _, ok := cfg.Hosts[host]; ok {
					return spaces.OpenCopy(cfg, host, session, opts)
				}
			}
			return spaces.OpenPath(cfg, dir, opts)
		},
	}
	cmd.ValidArgsFunction = func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		cfg, err := config.Load()
		if err != nil {
			return nil, cobra.ShellCompDirectiveDefault
		}
		return openCompletions(cfg, toComplete)
	}
	cmd.Flags().BoolVar(&noLayout, "no-layout", false, "open a session that has no windows and nothing saved to restore with one tab, not its default layout")
	return cmd
}

// openCompletions are what open's argument can be, starting with toComplete: this machine's worktrees, by
// path, and HOST: for each host the config names; after HOST:, that host's worktrees by their session
// there, as its server says.
func openCompletions(cfg config.Config, toComplete string) ([]string, cobra.ShellCompDirective) {
	var out []string
	add := func(c string) {
		if strings.HasPrefix(c, toComplete) {
			out = append(out, c)
		}
	}
	if host, _ := spaces.SplitCopy(toComplete); host != "" {
		if _, ok := cfg.Hosts[host]; !ok {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		h, hangUp, err := spaces.HostOf(cfg, host)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		defer hangUp()
		hd, err := h.Load(false)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		add(spaces.CopyName(host, ""))
		views, _ := spaces.RemoteViewsOf(map[string]spaces.RemoteHost{host: {Data: &hd}})
		for _, s := range views[host].Spaces {
			for _, wt := range s.Worktrees {
				add(wt.Session)
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Hosts)) {
		add(spaces.CopyName(name, ""))
	}
	if hd, err := spaces.ThisHost().Load(false); err == nil {
		for _, s := range hd.Spaces {
			for _, wt := range s.Worktrees {
				add(wt.Path)
			}
		}
	}
	// HOST: goes on to the host's sessions, so no space after it when it is all there is.
	if len(out) > 0 && !slices.ContainsFunc(out, func(c string) bool { return !strings.HasSuffix(c, ":") }) {
		return out, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
