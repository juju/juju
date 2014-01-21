// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statecmd

import (
	"fmt"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
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

// MachineConfig returns information from the environment config that is
// needed for machine cloud-init (both state servers and host nodes).
// The code is here so that it can be shared between the API server and the CLI
// for maintaining compatibiility with juju-1.16 (where the API MachineConfig
// did not exist). When we drop 1.16 compatibility, this code should be moved
// back into api.Client
func MachineConfig(st *state.State, machineId string) (params.MachineConfig, error) {
	result := params.MachineConfig{}
	environConfig, err := st.EnvironConfig()
	if err != nil {
		return result, err
	}
	// The basic information.
	result.EnvironAttrs = environConfig.AllAttrs()

	// Get the machine so we can get its series and arch.
	// If the Arch is not set in hardware-characteristics,
	// an error is returned.
	machine, err := st.Machine(machineId)
	if err != nil {
		return result, err
	}
	hc, err := machine.HardwareCharacteristics()
	if err != nil {
		return result, err
	}
	if hc.Arch == nil {
		return result, fmt.Errorf("arch is not set for %q", machine.Tag())
	}

	// Find the appropriate tools information.
	env, err := environs.New(environConfig)
	if err != nil {
		return result, err
	}
	tools, err := findInstanceTools(env, machine.Series(), *hc.Arch)
	if err != nil {
		return result, err
	}
	result.Tools = tools

	// Find the secrets and API endpoints.
	auth, err := environs.NewEnvironAuthenticator(env)
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

// FinishMachineConfig is shared by environs/manual and state/apiserver/client
// to create a complete cloudinit.MachineConfig from a params.MachineConfig.
func FinishMachineConfig(configParameters params.MachineConfig, machineId, nonce, dataDir string) (*cloudinit.MachineConfig, error) {
	stateInfo := &state.Info{
		Addrs:    configParameters.StateAddrs,
		Password: configParameters.Password,
		Tag:      configParameters.Tag,
		CACert:   configParameters.CACert,
	}
	apiInfo := &api.Info{
		Addrs:    configParameters.APIAddrs,
		Password: configParameters.Password,
		Tag:      configParameters.Tag,
		CACert:   configParameters.CACert,
	}
	environConfig, err := config.New(config.NoDefaults, configParameters.EnvironAttrs)
	if err != nil {
		return nil, err
	}
	mcfg := environs.NewMachineConfig(machineId, nonce, stateInfo, apiInfo)
	if dataDir != "" {
		mcfg.DataDir = dataDir
	}
	mcfg.Tools = configParameters.Tools
	err = environs.FinishMachineConfig(mcfg, environConfig, constraints.Value{})
	if err != nil {
		return nil, err
	}
	return mcfg, nil
}
