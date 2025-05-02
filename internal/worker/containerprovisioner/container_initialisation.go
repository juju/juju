// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerprovisioner

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/core/containermanager"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/container/broker"
	"github.com/juju/juju/internal/container/lxd"
	"github.com/juju/juju/rpc/params"
)

// ContainerProvisionerAPI describes methods for provisioning a container.
type ContainerProvisionerAPI interface {
	ContainerManagerConfigGetter
	broker.APICalls
}

// ContainerSetup sets up the machine to be able to create containers
// and start a suitable provisioner. Work is triggered by the
// ContainerSetupAndProvisioner.
type ContainerSetup struct {
	logger                  logger.Logger
	containerType           instance.ContainerType
	provisioner             ContainerProvisionerAPI
	controllerAPI           ControllerAPI
	machinesAPI             MachinesAPI
	toolsFinder             ToolsFinder
	distributionGroupFinder DistributionGroupFinder
	mTag                    names.MachineTag
	machineZone             broker.AvailabilityZoner
	config                  agent.Config
	machineLock             machinelock.Lock
	managerConfig           container.ManagerConfig

	getNetConfig func(network.ConfigSource) (network.InterfaceInfos, error)
}

// ContainerSetupParams are used to initialise a container setup worker.
type ContainerSetupParams struct {
	Logger        logger.Logger
	ContainerType instance.ContainerType
	MTag          names.MachineTag
	MachineZone   broker.AvailabilityZoner
	Provisioner   *apiprovisioner.Client
	Config        agent.Config
	MachineLock   machinelock.Lock
	GetNetConfig  func(network.ConfigSource) (network.InterfaceInfos, error)
}

// NewContainerSetup returns a ContainerSetup to start the container
// provisioner workers.
func NewContainerSetup(params ContainerSetupParams) *ContainerSetup {
	return &ContainerSetup{
		logger:                  params.Logger,
		containerType:           params.ContainerType,
		provisioner:             params.Provisioner,
		controllerAPI:           params.Provisioner,
		machinesAPI:             params.Provisioner,
		toolsFinder:             params.Provisioner,
		distributionGroupFinder: params.Provisioner,
		mTag:                    params.MTag,
		machineZone:             params.MachineZone,
		config:                  params.Config,
		machineLock:             params.MachineLock,
		getNetConfig:            params.GetNetConfig,
	}
}

func (cs *ContainerSetup) initialiseContainers(ctx context.Context, abort <-chan struct{}) error {
	cs.logger.Debugf(ctx, "setup for %s containers", cs.containerType)
	managerConfig, err := containerManagerConfig(ctx, cs.containerType, cs.provisioner)
	if err != nil {
		return errors.Annotate(err, "generating container manager config")
	}
	cs.managerConfig = managerConfig
	err = cs.initContainerDependencies(ctx, abort, managerConfig)
	return errors.Annotate(err, "setting up container dependencies on host machine")
}

// initContainerDependencies ensures that the host machine is set-up to manage
// containers of the input type.
func (cs *ContainerSetup) initContainerDependencies(ctx context.Context, abort <-chan struct{}, managerCfg container.ManagerConfig) error {
	snapChannels := map[string]string{
		"lxd": managerCfg.PopValue(config.LXDSnapChannel),
	}
	initialiser, err := getContainerInitialiser(
		cs.containerType,
		snapChannels,
		containermanager.NetworkingMethod(managerCfg.PopValue(config.ContainerNetworkingMethodKey)),
	)
	if err != nil {
		return errors.Annotate(err, "initialising container infrastructure on host machine")
	}

	releaser, err := cs.acquireLock(abort, fmt.Sprintf("%s container initialisation", cs.containerType))
	if err != nil {
		return errors.Annotate(err, "failed to acquire initialization lock")
	}
	defer releaser()

	if err := initialiser.Initialise(); err != nil {
		return errors.Trace(err)
	}

	// At this point, Initialiser likely has changed host network information,
	// so re-probe to have an accurate view.
	observedConfig, err := cs.observeNetwork()
	if err != nil {
		return errors.Annotate(err, "cannot discover observed network config")
	}
	if len(observedConfig) > 0 {
		machineTag := cs.mTag
		cs.logger.Tracef(ctx, "updating observed network config for %q %s containers to %#v",
			machineTag, cs.containerType, observedConfig)
		if err := cs.provisioner.SetHostMachineNetworkConfig(ctx, machineTag, observedConfig); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (cs *ContainerSetup) observeNetwork() ([]params.NetworkConfig, error) {
	interfaceInfos, err := cs.getNetConfig(network.DefaultConfigSource())
	if err != nil {
		return nil, err
	}
	return params.NetworkConfigFromInterfaceInfo(interfaceInfos), nil
}

func (cs *ContainerSetup) acquireLock(abort <-chan struct{}, comment string) (func(), error) {
	spec := machinelock.Spec{
		Cancel:  abort,
		Worker:  "container-provisioner",
		Comment: comment,
	}
	return cs.machineLock.Acquire(spec)
}

// getContainerInitialiser exists to patch out in tests.
var getContainerInitialiser = func(
	ct instance.ContainerType,
	snapChannels map[string]string,
	containerNetworkingMethod containermanager.NetworkingMethod,
) (container.Initialiser, error) {
	if ct != instance.LXD {
		return nil, errors.NotSupportedf("container type %q", ct)
	}
	return lxd.NewContainerInitialiser(snapChannels["lxd"], containerNetworkingMethod), nil
}

func (cs *ContainerSetup) initialiseContainerProvisioner(ctx context.Context) (Provisioner, error) {
	cs.logger.Debugf(ctx, "setup provisioner for %s containers", cs.containerType)
	if cs.managerConfig == nil {
		return nil, errors.NotValidf("Programming error, manager config not setup")
	}
	managerConfigWithZones, err := broker.ConfigureAvailabilityZone(ctx, cs.managerConfig, cs.machineZone)
	if err != nil {
		return nil, errors.Annotate(err, "configuring availability zones")
	}

	instanceBroker, err := broker.New(broker.Config{
		Name:          fmt.Sprintf("%s-provisioner", string(cs.containerType)),
		ContainerType: cs.containerType,
		ManagerConfig: managerConfigWithZones,
		APICaller:     cs.provisioner,
		AgentConfig:   cs.config,
		MachineTag:    cs.mTag,
		MachineLock:   cs.machineLock,
		GetNetConfig:  cs.getNetConfig,
	})
	if err != nil {
		return nil, errors.Annotate(err, "initialising container infrastructure on host machine")
	}

	w, err := NewContainerProvisioner(
		cs.containerType,
		cs.controllerAPI,
		cs.machinesAPI,
		cs.logger,
		cs.config,
		instanceBroker,
		cs.toolsFinder,
		cs.distributionGroupFinder,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func containerManagerConfig(
	ctx context.Context,
	containerType instance.ContainerType, configGetter ContainerManagerConfigGetter,
) (container.ManagerConfig, error) {
	// Ask the configGetter for the container manager configuration.
	managerConfigResult, err := configGetter.ContainerManagerConfig(
		ctx,
		params.ContainerManagerConfigParams{Type: containerType},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	managerConfig := container.ManagerConfig(managerConfigResult.ManagerConfig)
	return managerConfig, nil
}

type ContainerManagerConfigGetter interface {
	ContainerManagerConfig(context.Context, params.ContainerManagerConfigParams) (params.ContainerManagerConfig, error)
}
