// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"os"
	"path/filepath"
	"strings"

	jujuos "github.com/juju/juju/juju/os"
)

func osDependentEnvVars(paths Paths) []string {
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

// mergeEnvironment takes in a string array representing the desired environment
// and merges it with the current environment. On Windows, clearing the environment,
// or having missing environment variables, may lead to standard go packages not working
// (os.TempDir relies on $env:TEMP), and powershell erroring out
// TODO(fwereade, gsamfira): this is copy/pasted from utils/exec.
// This is only used on windows, so it is safe to do in a case insensitive way.
func mergeWindowsEnvironment(newEnv, env []string) []string {
	if len(newEnv) == 0 {
		return env
	}

	// this whole rigamarole is so that we retain the case of existing
	// environment variables, while being case insensitive about overwriting
	// their values.

	orig := make(map[string]string, len(env))
	uppers := make(map[string]string, len(env))
	news := map[string]string{}

	tmpEnv := make([]string, 0, len(env))
	for _, val := range env {
		varSplit := strings.SplitN(val, "=", 2)
		k := varSplit[0]
		uppers[strings.ToUpper(k)] = varSplit[1]
		orig[k] = varSplit[1]
	}

	for _, val := range newEnv {
		varSplit := strings.SplitN(val, "=", 2)
		k := varSplit[0]
		if _, ok := uppers[strings.ToUpper(k)]; ok {
			uppers[strings.ToUpper(k)] = varSplit[1]
		} else {
			news[k] = varSplit[1]
		}
	}

	for k, _ := range orig {
		tmpEnv = append(tmpEnv, k+"="+uppers[strings.ToUpper(k)])
	}

	for k, v := range news {
		tmpEnv = append(tmpEnv, k+"="+v)
	}
	return tmpEnv
}
