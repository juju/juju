// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statecmd

import (
	"fmt"

	"launchpad.net/juju-core/environs"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/worker/provisioner"
)
// TODO: Seems like a layering violation for the API server and CLI to want to
// import worker/provisioner code

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
// MachineConfig returns information from the environment config that is
// needed for machine cloud-init (both state servers and host nodes).
// The code is here so that it can be shared between the API server and the CLI
// for maintaining compatibiility with juju-1.16 (where the API MachineConfig
// did not exist). When we drop 1.16 compatibility, this code should be moved
// back into api.Client
func MachineConfig(st *state.State, args params.MachineConfigParams) (params.MachineConfig, error) {
	result := params.MachineConfig{}
	environConfig, err := st.EnvironConfig()
	if err != nil {
		return result, err
	}
	// The basic information.
	result.EnvironAttrs = environConfig.AllAttrs()

	// Find the appropriate tools information.
	env, err := environs.New(environConfig)
	if err != nil {
		return result, err
	}
	tools, err := findInstanceTools(env, args.Series, args.Arch)
	if err != nil {
		return result, err
	}
	result.Tools = tools

	// Find the secrets and API endpoints.
	auth, err := provisioner.NewEnvironAuthenticator(env)
	if err != nil {
		return result, err
	}
	machine, err := st.Machine(args.MachineId)
	if err != nil {
		return result, err
	}
	stateInfo, apiInfo, err := auth.SetupAuthentication(machine)
	if err != nil {
		return result, err
	}
	result.APIAddrs = apiInfo.Addrs
	result.StateAddrs = stateInfo.Addrs
	result.CACert = stateInfo.CACert
	result.Password = stateInfo.Password
	result.Tag = stateInfo.Tag
	return result, nil
}
