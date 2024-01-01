// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
)

var kvmLogger = loggo.GetLogger("juju.container.broker.kvm")

// NewKVMBroker creates a Broker that can be used to start KVM guests in a
// similar fashion to normal StartInstance requests.
// prepareHost is a callback that will be called when a new container is about
// to be started. It provides the intersection point where the host can update
// itself to be ready for whatever changes are necessary to have a functioning
// container. (such as bridging host devices.)
// manager is the infrastructure to actually launch the container.
// agentConfig is currently only used to find out the 'default' bridge to use
// when a specific network device is not specified in StartInstanceParams. This
// should be deprecated. And hopefully removed in the future.
func NewKVMBroker(
	prepareHost PrepareHostFunc,
	api APICalls,
	manager container.Manager,
	agentConfig agent.Config,
) (environs.InstanceBroker, error) {
	return &kvmBroker{
		prepareHost: prepareHost,
		manager:     manager,
		api:         api,
		agentConfig: agentConfig,
	}, nil
}

type kvmBroker struct {
	prepareHost PrepareHostFunc
	manager     container.Manager
	api         APICalls
	agentConfig agent.Config
}

// StartInstance is specified in the Broker interface.
func (broker *kvmBroker) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	// TODO: refactor common code out of the container brokers.
	containerMachineID := args.InstanceConfig.MachineId
	kvmLogger.Infof("starting kvm container for containerMachineID: %s", containerMachineID)

	config, err := broker.api.ContainerConfig()
	if err != nil {
		kvmLogger.Errorf("failed to get container config: %v", err)
		return nil, err
	}

	err = broker.prepareHost(names.NewMachineTag(containerMachineID), kvmLogger, args.Abort)
	if err != nil {
		return nil, errors.Trace(err)
	}

	preparedInfo, err := prepareContainerInterfaceInfo(broker.api, containerMachineID, kvmLogger)
	if err != nil {
		return nil, errors.Trace(err)
	}

	interfaces, err := finishNetworkConfig(preparedInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	net := container.BridgeNetworkConfig(0, interfaces)

	// The provisioner worker will provide all tools it knows about
	// (after applying explicitly specified constraints), which may
	// include tools for architectures other than the host's.
	//
	// container/kvm only allows running container==host arch, so
	// we constrain the tools to host arch here regardless of the
	// constraints specified.
	archTools, err := matchHostArchTools(args.Tools)
	if err != nil {
		return nil, errors.Trace(err)
	}

	args.InstanceConfig.MachineContainerType = instance.KVM
	if err := args.InstanceConfig.SetTools(archTools); err != nil {
		return nil, errors.Trace(err)
	}

	cloudInitUserData, err := combinedCloudInitData(
		config.CloudInitUserData,
		config.ContainerInheritProperties,
		args.InstanceConfig.Base, kvmLogger)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := instancecfg.PopulateInstanceConfig(
		args.InstanceConfig,
		config.ProviderType,
		config.AuthorizedKeys,
		config.SSLHostnameVerification,
		proxyConfigurationFromContainerCfg(config),
		config.EnableOSRefreshUpdate,
		config.EnableOSUpgrade,
		cloudInitUserData,
		nil,
	); err != nil {
		kvmLogger.Errorf("failed to populate machine config: %v", err)
		return nil, err
	}

	storageConfig := &container.StorageConfig{
		AllowMount: true,
	}
	inst, hardware, err := broker.manager.CreateContainer(
		ctx, args.InstanceConfig, args.Constraints, args.InstanceConfig.Base, net, storageConfig, args.StatusCallback,
	)
	if err != nil {
		kvmLogger.Errorf("failed to start container: %v", err)
		return nil, err
	}
	kvmLogger.Infof("started kvm container for containerMachineID: %s, %s, %s", containerMachineID, inst.Id(), hardware.String())
	return &environs.StartInstanceResult{
		Instance: inst,
		Hardware: hardware,
	}, nil
}

// StopInstances shuts down the given instances.
func (broker *kvmBroker) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	for _, id := range ids {
		kvmLogger.Infof("stopping kvm container for instance: %s", id)
		if err := broker.manager.DestroyContainer(id); err != nil {
			kvmLogger.Errorf("container did not stop: %v", err)
			return err
		}
		releaseContainerAddresses(broker.api, id, broker.manager.Namespace(), kvmLogger)
	}
	return nil
}

// AllInstances returns all containers.
func (broker *kvmBroker) AllInstances(ctx context.ProviderCallContext) (result []instances.Instance, err error) {
	return broker.manager.ListContainers()
}

// AllRunningInstances only returns running containers.
func (broker *kvmBroker) AllRunningInstances(ctx context.ProviderCallContext) (result []instances.Instance, err error) {
	return broker.manager.ListContainers()
}
