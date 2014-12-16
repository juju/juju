// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/juju/version"
)

func osDependentEnvVars(paths Paths) []string {
	switch version.Current.OS {
	case version.Windows:
		return windowsEnv(paths)
	case version.Ubuntu:
		return ubuntuEnv(paths)
	case version.CentOS:
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

// mergeEnvironment takes in a string array representing the desired environment
// and merges it with the current environment. On Windows, clearing the environment,
// or having missing environment variables, may lead to standard go packages not working
// (os.TempDir relies on $env:TEMP), and powershell erroring out
// TODO(fwereade, gsamfira): this is copy/pasted from utils/exec.
func mergeEnvironment(env []string) []string {
	if env == nil {
		return nil
	}
	m := map[string]string{}
	var tmpEnv []string
	for _, val := range os.Environ() {
		varSplit := strings.SplitN(val, "=", 2)
		m[varSplit[0]] = varSplit[1]
	}

	for _, val := range env {
		varSplit := strings.SplitN(val, "=", 2)
		m[varSplit[0]] = varSplit[1]
	}

	for key, val := range m {
		tmpEnv = append(tmpEnv, key+"="+val)
	}

	return tmpEnv
}
