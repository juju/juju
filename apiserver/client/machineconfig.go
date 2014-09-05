// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/state"
)

// MachineConfig returns information from the environment config that
// is needed for machine cloud-init (for non-state servers only). It
// is exposed for testing purposes.
// TODO(rog) fix environs/manual tests so they do not need to call this, or move this elsewhere.
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
	agentVersion, ok := env.Config().AgentVersion()
	if !ok {
		return nil, errors.New("no agent version set in environment configuration")
	}
	tools, err := envtools.FindExactTools(env, agentVersion, machine.Series(), *hc.Arch)
	if err != nil {
		return nil, err
	}

	// Find the API endpoints.
	apiInfo, err := environs.APIInfo(env)
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

	mcfg, err := environs.NewMachineConfig(machineId, nonce, env.Config().ImageStream(), machine.Series(), networks, mongoInfo, apiInfo)
	if err != nil {
		return nil, err
	}
	if dataDir != "" {
		mcfg.DataDir = dataDir
	}
	mcfg.Tools = tools
	err = environs.FinishMachineConfig(mcfg, environConfig)
	if err != nil {
		return nil, err
	}
	return mcfg, nil
}
