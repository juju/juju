// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"os"
	"path/filepath"

	jujuos "github.com/juju/utils/os"
)

// OSDependentEnvVars returns the OS-dependent environment variables that
// should be set for a hook context.
func OSDependentEnvVars(paths Paths) []string {
	switch jujuos.HostOS() {
	case jujuos.Windows:
		return windowsEnv(paths)
	case jujuos.Ubuntu:
		return ubuntuEnv(paths)
	case jujuos.CentOS:
		return centosEnv(paths)
	}
	return nil
}

func appendPath(paths Paths) []string {
	return []string{
		"PATH=" + paths.GetToolsDir() + ":" + os.Getenv("PATH"),
	}
}

func ubuntuEnv(paths Paths) []string {
	path := appendPath(paths)
	env := []string{
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
	}
	env = append(env, path...)
	return env
}

func centosEnv(paths Paths) []string {
	return appendPath(paths)
}

// windowsEnv adds windows specific environment variables. PSModulePath
// helps hooks use normal imports instead of dot sourcing modules
// its a convenience variable. The PATH variable delimiter is
// a semicolon instead of a colon
func windowsEnv(paths Paths) []string {
	charmDir := paths.GetCharmDir()
	charmModules := filepath.Join(charmDir, "lib", "Modules")
	return []string{
		"Path=" + paths.GetToolsDir() + ";" + os.Getenv("Path"),
		"PSModulePath=" + os.Getenv("PSModulePath") + ";" + charmModules,
	}
}
