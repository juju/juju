// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"strings"
)

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
