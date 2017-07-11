// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/controller/authentication"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// InstanceConfig returns information from the environment config that
// is needed for machine cloud-init (for non-controllers only). It
// is exposed for testing purposes.
// TODO(rog) fix environs/manual tests so they do not need to call this, or move this elsewhere.
func InstanceConfig(st *state.State, machineId, nonce, dataDir string) (*instancecfg.InstanceConfig, error) {
	modelConfig, err := st.ModelConfig()
	if err != nil {
		return nil, errors.Annotate(err, "getting model config")
	}

	// Get the machine so we can get its series and arch.
	// If the Arch is not set in hardware-characteristics,
	// an error is returned.
	machine, err := st.Machine(machineId)
	if err != nil {
		return nil, errors.Annotate(err, "getting machine")
	}
	hc, err := machine.HardwareCharacteristics()
	if err != nil {
		return nil, errors.Annotate(err, "getting machine hardware characteristics")
	}
	if hc.Arch == nil {
		return nil, fmt.Errorf("arch is not set for %q", machine.Tag())
	}

	// Find the appropriate tools information.
	agentVersion, ok := modelConfig.AgentVersion()
	if !ok {
		return nil, errors.New("no agent version set in model configuration")
	}
	environment, err := st.Model()
	if err != nil {
		return nil, errors.Annotate(err, "getting state model")
	}
	urlGetter := common.NewToolsURLGetter(environment.UUID(), st)
	configGetter := stateenvirons.EnvironConfigGetter{st}
	toolsFinder := common.NewToolsFinder(configGetter, st, urlGetter)
	findToolsResult, err := toolsFinder.FindTools(params.FindToolsParams{
		Number:       agentVersion,
		MajorVersion: -1,
		MinorVersion: -1,
		Series:       machine.Series(),
		Arch:         *hc.Arch,
	})
	if err != nil {
		return nil, errors.Annotate(err, "finding tools")
	}
	if findToolsResult.Error != nil {
		return nil, errors.Annotate(findToolsResult.Error, "finding tools")
	}
	toolsList := findToolsResult.List

	// Get the API connection info; attempt all API addresses.
	apiHostPorts, err := st.APIHostPorts()
	if err != nil {
		return nil, errors.Annotate(err, "getting API addresses")
	}
	apiAddrs := make(set.Strings)
	for _, hostPorts := range apiHostPorts {
		for _, hp := range hostPorts {
			apiAddrs.Add(hp.NetAddr())
		}
	}
	apiInfo := &api.Info{
		Addrs:    apiAddrs.SortedValues(),
		CACert:   st.CACert(),
		ModelTag: st.ModelTag(),
	}

	auth := authentication.NewAuthenticator(st.MongoConnectionInfo(), apiInfo)
	_, apiInfo, err = auth.SetupAuthentication(machine)
	if err != nil {
		return nil, errors.Annotate(err, "setting up machine authentication")
	}

	icfg, err := instancecfg.NewInstanceConfig(st.ControllerTag(), machineId, nonce, modelConfig.ImageStream(),
		machine.Series(), apiInfo,
	)
	if err != nil {
		return nil, errors.Annotate(err, "initializing instance config")
	}
	if dataDir != "" {
		icfg.DataDir = dataDir
	}
	if err := icfg.SetTools(toolsList); err != nil {
		return nil, errors.Trace(err)
	}
	err = instancecfg.FinishInstanceConfig(icfg, modelConfig)
	if err != nil {
		return nil, errors.Annotate(err, "finishing instance config")
	}
	return icfg, nil
}
