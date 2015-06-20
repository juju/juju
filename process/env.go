// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"strings"

	"github.com/juju/errors"
)

// ParseEnv converts the provided strings into a mapping of environment
// variable names to values. The entries must be formatted as
// "ENV_VAR=value", "ENV_VAR=", or just "ENV_VAR". The equal sign is
// implied if missing.
func ParseEnv(raw []string) (map[string]string, error) {
	envVars := make(map[string]string)
	for _, envVarStr := range raw {
		envVarStr = strings.TrimSpace(envVarStr)
		if envVarStr == "" {
			continue
		}
		parts := strings.SplitN(envVarStr, "=", 2)
		if len(parts) == 1 {
			parts = append(parts, "")
		}
		envVar, value := parts[0], parts[1]
		if envVar == "" {
			return nil, errors.Errorf(`got "" for env var name`)
		}
		envVars[envVar] = value
	}
	return envVars, nil
}

// UnparseEnv converts the provided environment variables into the
// format expected by ParseEnv.
func UnparseEnv(env map[string]string) ([]string, error) {
	var envVars []string
	for k, v := range env {
		if k == "" {
			return nil, errors.Errorf(`got "" for env var name`)
		}
		envVars = append(envVars, k+"="+v)
	}
	return envVars, nil
}
