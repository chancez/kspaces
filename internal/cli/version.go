package cli

import "runtime/debug"

// version is set by a release build, with -ldflags "-X github.com/chancez/kspaces/internal/cli.version=v1.2.3".
var version string

// versionOf is kspaces' version: a release build's, the module's for go install, or dev for a build from a
// checkout.
func versionOf(release string, info *debug.BuildInfo, ok bool) string {
	switch {
	case release != "":
		return release
	case ok && info.Main.Version != "" && info.Main.Version != "(devel)":
		return info.Main.Version
	}
	return "dev"
}
