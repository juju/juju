// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/state"
	"github.com/juju/juju/tools"
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
// It is exposed for testing purposes.
// TODO(rog) fix environs/manual tests so they do not need to
// call this, or move this elsewhere.
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

	// Find the API endpoints.
	_, apiInfo, err := env.StateInfo()
	if err != nil {
		return nil, err
	}

	auth := authentication.NewAuthenticator(st.MongoConnectionInfo(), apiInfo)
	mongoInfo, apiInfo, err := auth.SetupAuthentication(machine)
	if err != nil {
		return nil, err
	}

	// Find requested networks.
	networks, err := machine.RequestedNetworks()
	if err != nil {
		return nil, err
	}

	mcfg := environs.NewMachineConfig(machineId, nonce, networks, mongoInfo, apiInfo)
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
