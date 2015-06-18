// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"encoding/json"
	"strings"

	"github.com/juju/errors"
)

// LaunchDetails holds information about an existing process as provided by
// a workload process plugin.
type LaunchDetails struct {
	// ID is provided by the plugin as a guaranteed way
	// to uniquely identify the process to the plugin.
	ID string `json:"id"`

	// Status is the status of the process as reported by the plugin.
	Status string `json:"status"`
}

// Validate returns an error if LaunchDetails is not well-formed.
func (ld LaunchDetails) Validate() error {
	if ld.ID == "" {
		return errors.Errorf("ID must be set")
	}
	if ld.Status == "" {
		return errors.Errorf("Status must be set")
	}
	return nil
}

// ParseDetails parses the input string in to a LaunchDetails struct.
// ParseDetails expects the plugin to return JSON.
func ParseDetails(input string) (*LaunchDetails, error) {
	var details LaunchDetails
	if err := json.Unmarshal([]byte(input), &details); err != nil {
		if strings.TrimSpace(input) != "" {
			return nil, errors.Trace(err)
		}
	}
	if err := details.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return &details, nil
}

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
