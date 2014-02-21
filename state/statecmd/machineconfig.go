// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statecmd

import (
	"fmt"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/state"
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
// needed for machine cloud-init (for non-state servers only).
//
// The code is here so that it can be shared between the API server
// (for ProvisioningScript) and the CLI (for manual provisioning) for maintaining
// compatibiility with juju-1.16 (where the API MachineConfig did not exist).
// When we drop 1.16 compatibility, this code should be moved into apiserver.
func MachineConfig(st *state.State, machineId, nonce, dataDir string) (*cloudinit.MachineConfig, error) {
	environConfig, err := st.EnvironConfig()
	if err != nil {
		return nil, err
	}

	// Get the machine so we can get its series and arch.
	// If the Arch is not set in hardware-characteristics,
	// an error is returned.
	machine, err := st.Machine(machineId)
	if err != nil {
		return nil, err
	}
	hc, err := machine.HardwareCharacteristics()
	if err != nil {
		return nil, err
	}
	if hc.Arch == nil {
		return nil, fmt.Errorf("arch is not set for %q", machine.Tag())
	}

	// Find the appropriate tools information.
	env, err := environs.New(environConfig)
	if err != nil {
		return nil, err
	}
	tools, err := findInstanceTools(env, machine.Series(), *hc.Arch)
	if err != nil {
		return nil, err
	}

	// Find the secrets and API endpoints.
	auth, err := environs.NewEnvironAuthenticator(env)
	if err != nil {
		return nil, err
	}
	stateInfo, apiInfo, err := auth.SetupAuthentication(machine)
	if err != nil {
		return nil, err
	}

	mcfg := environs.NewMachineConfig(machineId, nonce, stateInfo, apiInfo)
	if dataDir != "" {
		mcfg.DataDir = dataDir
	}
	mcfg.Tools = tools
	err = environs.FinishMachineConfig(mcfg, environConfig, constraints.Value{})
	if err != nil {
		return nil, err
	}
	return mcfg, nil
}
