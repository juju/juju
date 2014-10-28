// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/juju/version"
)

// hookVars returns an os.Environ-style list of strings necessary to run a hook
// such that it can know what environment it's operating in, and can call back
// into context.
func hookVars(context *HookContext, paths Paths) []string {
	// TODO(binary132): add Action env variables: JUJU_ACTION_NAME,
	// JUJU_ACTION_UUID, ...
	vars := context.proxySettings.AsEnvironmentValues()
	vars = append(vars,
		"CHARM_DIR="+paths.GetCharmDir(), // legacy, embarrassing
		"JUJU_CHARM_DIR="+paths.GetCharmDir(),
		"JUJU_CONTEXT_ID="+context.id,
		"JUJU_AGENT_SOCKET="+paths.GetJujucSocket(),
		"JUJU_UNIT_NAME="+context.unitName,
		"JUJU_ENV_UUID="+context.uuid,
		"JUJU_ENV_NAME="+context.envName,
		"JUJU_API_ADDRESSES="+strings.Join(context.apiAddrs, " "),
		"JUJU_METER_STATUS="+context.meterStatus.code,
		"JUJU_METER_INFO="+context.meterStatus.info,
	)
	if r, found := context.HookRelation(); found {
		vars = append(vars,
			"JUJU_RELATION="+r.Name(),
			"JUJU_RELATION_ID="+r.FakeId(),
			"JUJU_REMOTE_UNIT="+context.remoteUnitName,
		)
	}
	return append(vars, osDependentEnvVars(paths)...)
}

func osDependentEnvVars(paths Paths) []string {
	switch version.Current.OS {
	case version.Windows:
		return windowsEnv(paths)
	case version.Ubuntu:
		return ubuntuEnv(paths)
	}
	return nil
}

func ubuntuEnv(paths Paths) []string {
	env := []string{
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
		"PATH=" + paths.GetToolsDir() + ":" + os.Getenv("PATH"),
	}
	return env
}

// windowsEnv adds windows specific environment variables. PSModulePath
// helps hooks use normal imports instead of dot sourcing modules
// its a convenience variable. The PATH variable delimiter is
// a semicolon instead of a colon
func windowsEnv(paths Paths) []string {
	charmDir := paths.GetCharmDir()
	// TODO(fwereade, gsamfira): if anything we should just use <charm>/lib/Modules
	charmModules := filepath.Join(charmDir, "Modules")
	hookModules := filepath.Join(charmDir, "hooks", "Modules")
	return []string{
		"Path=" + paths.GetToolsDir() + ";" + os.Getenv("Path"),
		"PSModulePath=" + os.Getenv("PSModulePath") + ";" + charmModules + ";" + hookModules,
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
