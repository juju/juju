// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"

	"launchpad.net/juju-core/environs"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/tools"
)

func findInstanceTools(env environs.Environ, series, arch string) (*tools.Tools, error) {
	agentVersion, ok := env.Config().AgentVersion()
	if !ok {
		return nil, fmt.Errorf("no agent version set in environment configuration")
	}
	possibleTools, err := envtools.FindInstanceTools(env, agentVersion, series, &arch)
	if err != nil {
		return nil, err
	}
	return possibleTools[0], nil
}
