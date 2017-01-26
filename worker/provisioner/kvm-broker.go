// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

var kvmLogger = loggo.GetLogger("juju.provisioner.kvm")

func NewKvmBroker(
	bridger network.Bridger,
	hostMachineID string,
	api APICalls,
	agentConfig agent.Config,
	managerConfig container.ManagerConfig,
) (environs.InstanceBroker, error) {
	manager, err := kvm.NewContainerManager(managerConfig)
	if err != nil {
		return nil, err
	}
	return &kvmBroker{
		bridger:       bridger,
		hostMachineID: hostMachineID,
		manager:       manager,
		api:           api,
		agentConfig:   agentConfig,
	}, nil
}

type kvmBroker struct {
	bridger       network.Bridger
	hostMachineID string
	manager       container.Manager
	api           APICalls
	agentConfig   agent.Config
}

// StartInstance is specified in the Broker interface.
func (broker *kvmBroker) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	// TODO: refactor common code out of the container brokers.
	containerMachineID := args.InstanceConfig.MachineId
	kvmLogger.Infof("starting kvm container for containerMachineID: %s", containerMachineID)

	// TODO: Default to using the host network until we can configure.  Yes,
	// this is using the LxcBridge value, we should put it in the api call for
	// container config.
	bridgeDevice := broker.agentConfig.Value(agent.LxcBridge)
	if bridgeDevice == "" {
		bridgeDevice = container.DefaultKvmBridge
	}

	config, err := broker.api.ContainerConfig()
	if err != nil {
		kvmLogger.Errorf("failed to get container config: %v", err)
		return nil, err
	}

	err = prepareHost(broker.bridger, broker.hostMachineID, names.NewMachineTag(containerMachineID), broker.api, kvmLogger)
	if err != nil {
		return nil, errors.Trace(err)
	}

	preparedInfo, err := prepareOrGetContainerInterfaceInfo(
		broker.api,
		containerMachineID,
		bridgeDevice,
		true, // allocate if possible, do not maintain existing.
		kvmLogger,
	)
	if err != nil {
		// It's not fatal (yet) if we couldn't pre-allocate addresses for the
		// container.
		logger.Warningf("failed to prepare container %q network config: %v", containerMachineID, err)
	} else {
		args.NetworkInfo = preparedInfo
	}

	network := container.BridgeNetworkConfig(bridgeDevice, 0, args.NetworkInfo)
	interfaces, err := finishNetworkConfig(bridgeDevice, args.NetworkInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	network.Interfaces = interfaces

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

	series := archTools.OneSeries()
	args.InstanceConfig.MachineContainerType = instance.KVM
	if err := args.InstanceConfig.SetTools(archTools); err != nil {
		return nil, errors.Trace(err)
	}

	if err := instancecfg.PopulateInstanceConfig(
		args.InstanceConfig,
		config.ProviderType,
		config.AuthorizedKeys,
		config.SSLHostnameVerification,
		config.Proxy,
		config.AptProxy,
		config.AptMirror,
		config.EnableOSRefreshUpdate,
		config.EnableOSUpgrade,
	); err != nil {
		kvmLogger.Errorf("failed to populate machine config: %v", err)
		return nil, err
	}

	storageConfig := &container.StorageConfig{
		AllowMount: true,
	}
	inst, hardware, err := broker.manager.CreateContainer(
		args.InstanceConfig, args.Constraints,
		series, network, storageConfig, args.StatusCallback,
	)
	if err != nil {
		kvmLogger.Errorf("failed to start container: %v", err)
		return nil, err
	}
	kvmLogger.Infof("started kvm container for containerMachineID: %s, %s, %s", containerMachineID, inst.Id(), hardware.String())
	return &environs.StartInstanceResult{
		Instance:    inst,
		Hardware:    hardware,
		NetworkInfo: interfaces,
	}, nil
}

// MaintainInstance ensures the container's host has the required iptables and
// routing rules to make the container visible to both the host and other
// machines on the same subnet.
func (broker *kvmBroker) MaintainInstance(args environs.StartInstanceParams) error {
	machineID := args.InstanceConfig.MachineId

	// Default to using the host network until we can configure.
	bridgeDevice := broker.agentConfig.Value(agent.LxcBridge)
	if bridgeDevice == "" {
		bridgeDevice = container.DefaultKvmBridge
	}

	// There's no InterfaceInfo we expect to get below.
	_, err := prepareOrGetContainerInterfaceInfo(
		broker.api,
		machineID,
		bridgeDevice,
		false, // maintain, do not allocate.
		kvmLogger,
	)
	return err
}

// StopInstances shuts down the given instances.
func (broker *kvmBroker) StopInstances(ids ...instance.Id) error {
	// TODO: potentially parallelise.
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

// AllInstances only returns running containers.
func (broker *kvmBroker) AllInstances() (result []instance.Instance, err error) {
	return broker.manager.ListContainers()
}
