// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/controller/authentication"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/state/stateenvirons"
)

type InstanceConfigBackend interface {
	Model() (Model, error)
	Machine(string) (Machine, error)
	ToolsStorage() (binarystorage.StorageCloser, error)
}

// InstanceConfig returns information from the model config that
// is needed for machine cloud-init (for non-controllers only). It
// is exposed for testing purposes.
// TODO(rog) fix environs/manual tests so they do not need to call this, or move this elsewhere.
func InstanceConfig(ctrlSt ControllerBackend, st InstanceConfigBackend, machineId, nonce, dataDir string) (*instancecfg.InstanceConfig, error) {
	model, err := st.Model()
	if err != nil {
		return nil, errors.Annotate(err, "getting state model")
	}
	modelConfig, err := model.Config()
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
	urlGetter := common.NewToolsURLGetter(model.UUID(), ctrlSt)
	configGetter := stateenvirons.EnvironConfigGetter{Model: model}
	newEnviron := func() (environs.BootstrapEnviron, error) {
		return environs.GetEnviron(configGetter, environs.New)
	}
	toolsFinder := common.NewToolsFinder(configGetter, st, urlGetter, newEnviron)
	toolsList, err := toolsFinder.FindAgents(common.FindAgentsParams{
		Number: agentVersion,
		OSType: machine.Base().OS,
		Arch:   *hc.Arch,
	})
	if err != nil {
		return nil, errors.Annotate(err, "finding agent binaries")
	}

	controllerConfig, err := ctrlSt.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	caCert, _ := controllerConfig.CACert()

	// Get the API connection info; attempt all API addresses.
	apiHostPorts, err := ctrlSt.APIHostPortsForAgents()
	if err != nil {
		return nil, errors.Annotate(err, "getting API addresses")
	}
	apiAddrs := make(set.Strings)
	for _, hostPorts := range apiHostPorts {
		for _, hp := range hostPorts {
			apiAddrs.Add(network.DialAddress(hp))
		}
	}
	apiInfo := &api.Info{
		Addrs:    apiAddrs.SortedValues(),
		CACert:   caCert,
		ModelTag: model.ModelTag(),
	}

	apiInfo, err = authentication.SetupAuthentication(machine, apiInfo)
	if err != nil {
		return nil, errors.Annotate(err, "setting up machine authentication")
	}

	base, err := corebase.ParseBase(machine.Base().OS, machine.Base().Channel)
	if err != nil {
		return nil, errors.Annotate(err, "getting machine base")
	}
	icfg, err := instancecfg.NewInstanceConfig(ctrlSt.ControllerTag(), machineId, nonce, modelConfig.ImageStream(),
		base, apiInfo,
	)
	if err != nil {
		return nil, errors.Annotate(err, "initializing instance config")
	}

	icfg.ControllerConfig = make(map[string]interface{})
	for k, v := range controllerConfig {
		icfg.ControllerConfig[k] = v
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
