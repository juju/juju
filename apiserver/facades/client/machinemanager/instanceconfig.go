// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/common"
	corebase "github.com/juju/juju/core/base"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	agentbinaryservice "github.com/juju/juju/domain/agentbinary/service"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/state/binarystorage"
)

type InstanceConfigBackend interface {
	Machine(string) (Machine, error)
	ToolsStorage(objectstore.ObjectStore) (binarystorage.StorageCloser, error)
}

// AgentBinaryService is an interface for getting the
// EnvironAgentBinariesFinder function.
type AgentBinaryService interface {
	// GetEnvironAgentBinariesFinder returns the function to find agent binaries.
	// This is used to find the agent binaries.
	GetEnvironAgentBinariesFinder() agentbinaryservice.EnvironAgentBinariesFinderFunc
}

// AgentPasswordService defines the methods required to set an agent password
// hash.
type AgentPasswordService interface {
	// SetMachinePassword sets the password hash for the given machine.
	SetMachinePassword(context.Context, coremachine.Name, string) error
}

// InstanceConfigServices holds the services needed to configure instances.
type InstanceConfigServices struct {
	ControllerConfigService ControllerConfigService
	CloudService            common.CloudService
	KeyUpdaterService       KeyUpdaterService
	ModelConfigService      ModelConfigService
	MachineService          MachineService
	ObjectStore             objectstore.ObjectStore
	AgentBinaryService      AgentBinaryService
	AgentPasswordService    AgentPasswordService
}

// InstanceConfig returns information from the model config that
// is needed for configuring manual machines.
// It is exposed for testing purposes.
func InstanceConfig(
	ctx context.Context,
	modelID coremodel.UUID,
	providerGetter func(context.Context) (environs.BootstrapEnviron, error),
	ctrlSt ControllerBackend,
	st InstanceConfigBackend,
	services InstanceConfigServices,
	machineId, nonce, dataDir string,
) (*instancecfg.InstanceConfig, error) {
	modelConfig, err := services.ModelConfigService.ModelConfig(ctx)
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

	machineName := coremachine.Name(machineId)
	machineUUID, err := services.MachineService.GetMachineUUID(ctx, coremachine.Name(machineId))
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving machine UUID for machine %q", machineId)
	}
	hc, err := services.MachineService.GetHardwareCharacteristics(ctx, machineUUID)
	if err != nil {
		return nil, errors.Annotate(err, "getting machine hardware characteristics")
	}
	if hc.Arch == nil {
		return nil, fmt.Errorf("arch is not set for %q", machineName)
	}

	controllerConfigService := services.ControllerConfigService
	controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Find the appropriate tools information.
	agentVersion, ok := modelConfig.AgentVersion()
	if !ok {
		return nil, errors.New("no agent version set in model configuration")
	}
	urlGetter := common.NewToolsURLGetter(modelID.String(), ctrlSt)
	toolsFinder := common.NewToolsFinder(
		services.ControllerConfigService,
		st,
		urlGetter,
		services.ObjectStore,
		services.AgentBinaryService,
	)
	toolsList, err := toolsFinder.FindAgents(ctx, common.FindAgentsParams{
		Number: agentVersion,
		OSType: machine.Base().OS,
		Arch:   *hc.Arch,
	})
	if err != nil {
		return nil, errors.Annotate(err, "finding agent binaries")
	}

	// Get the API connection info; attempt all API addresses.
	apiHostPorts, err := ctrlSt.APIHostPortsForAgents(controllerConfig)
	if err != nil {
		return nil, errors.Annotate(err, "getting API addresses")
	}
	apiAddrs := make(set.Strings)
	for _, hostPorts := range apiHostPorts {
		for _, hp := range hostPorts {
			apiAddrs.Add(network.DialAddress(hp))
		}
	}

	password, err := password.RandomPassword()
	if err != nil {
		return nil, fmt.Errorf("cannot make password for machine %v: %v", machineName, err)
	}
	if err := services.AgentPasswordService.SetMachinePassword(ctx, machineName, password); errors.Is(err, applicationerrors.MachineNotFound) {
		return nil, errors.NotFoundf("machine %q", machineId)
	} else if err != nil {
		return nil, fmt.Errorf("setting password for machine %v: %v", machineName, err)
	}

	caCert, _ := controllerConfig.CACert()
	apiInfo := &api.Info{
		Addrs:    apiAddrs.SortedValues(),
		CACert:   caCert,
		ModelTag: names.NewModelTag(modelID.String()),
		Tag:      machine.Tag(),
		Password: password,
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

	keys, err := services.KeyUpdaterService.GetAuthorisedKeysForMachine(
		ctx,
		coremachine.Name(machineId),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"cannot get authorised keys for machine %q while generating instance config: %w",
			machineId, err,
		)
	}
	icfg.AuthorizedKeys = strings.Join(keys, "\n")

	return icfg, nil
}
