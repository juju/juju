// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
)

var kvmLogger = loggo.GetLogger("juju.provisioner.kvm")

var _ environs.InstanceBroker = (*kvmBroker)(nil)

func NewKvmBroker(
	api APICalls,
	agentConfig agent.Config,
	managerConfig container.ManagerConfig,
	enableNAT bool,
) (environs.InstanceBroker, error) {
	manager, err := kvm.NewContainerManager(managerConfig)
	if err != nil {
		return nil, err
	}
	return &kvmBroker{
		manager:     manager,
		api:         api,
		agentConfig: agentConfig,
		enableNAT:   enableNAT,
	}, nil
}

type kvmBroker struct {
	manager     container.Manager
	api         APICalls
	agentConfig agent.Config
	enableNAT   bool
}

// StartInstance is specified in the Broker interface.
func (broker *kvmBroker) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	if args.InstanceConfig.HasNetworks() {
		return nil, errors.New("starting kvm containers with networks is not supported yet")
	}
	// TODO: refactor common code out of the container brokers.
	machineId := args.InstanceConfig.MachineId
	kvmLogger.Infof("starting kvm container for machineId: %s", machineId)

	// TODO: Default to using the host network until we can configure.  Yes,
	// this is using the LxcBridge value, we should put it in the api call for
	// container config.
	bridgeDevice := broker.agentConfig.Value(agent.LxcBridge)
	if bridgeDevice == "" {
		bridgeDevice = kvm.DefaultKvmBridge
	}
	if !environs.AddressAllocationEnabled() {
		logger.Debugf(
			"address allocation feature flag not enabled; using DHCP for container %q",
			machineId,
		)
	} else {
		logger.Debugf("trying to allocate static IP for container %q", machineId)

		allocatedInfo, err := configureContainerNetwork(
			machineId,
			bridgeDevice,
			broker.api,
			args.NetworkInfo,
			true, // allocate a new address.
			broker.enableNAT,
		)
		if err != nil {
			// It's fine, just ignore it. The effect will be that the
			// container won't have a static address configured.
			logger.Infof("not allocating static IP for container %q: %v", machineId, err)
		} else {
			args.NetworkInfo = allocatedInfo
		}
	}

	// Unlike with LXC, we don't override the default MTU to use.
	network := container.BridgeNetworkConfig(bridgeDevice, 0, args.NetworkInfo)

	series := args.Tools.OneSeries()
	args.InstanceConfig.MachineContainerType = instance.KVM
	args.InstanceConfig.Tools = args.Tools[0]

	config, err := broker.api.ContainerConfig()
	if err != nil {
		kvmLogger.Errorf("failed to get container config: %v", err)
		return nil, err
	}

	if err := instancecfg.PopulateInstanceConfig(
		args.InstanceConfig,
		config.ProviderType,
		config.AuthorizedKeys,
		config.SSLHostnameVerification,
		config.Proxy,
		config.AptProxy,
		config.AptMirror,
		config.PreferIPv6,
		config.EnableOSRefreshUpdate,
		config.EnableOSUpgrade,
	); err != nil {
		kvmLogger.Errorf("failed to populate machine config: %v", err)
		return nil, err
	}

	storageConfig := &container.StorageConfig{
		AllowMount: true,
	}
	inst, hardware, err := broker.manager.CreateContainer(args.InstanceConfig, series, network, storageConfig)
	if err != nil {
		kvmLogger.Errorf("failed to start container: %v", err)
		return nil, err
	}
	kvmLogger.Infof("started kvm container for machineId: %s, %s, %s", machineId, inst.Id(), hardware.String())
	return &environs.StartInstanceResult{
		Instance:    inst,
		Hardware:    hardware,
		NetworkInfo: network.Interfaces,
	}, nil
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
	}
	return nil
}

// AllInstances only returns running containers.
func (broker *kvmBroker) AllInstances() (result []instance.Instance, err error) {
	return broker.manager.ListContainers()
}

// MaintainInstance checks that the container's host has the required iptables and routing
// rules to make the container visible to both the host and other machines on the same subnet.
func (broker *kvmBroker) MaintainInstance(args environs.StartInstanceParams) error {
	machineId := args.InstanceConfig.MachineId
	if !environs.AddressAllocationEnabled() {
		kvmLogger.Debugf("address allocation disabled: Not running maintenance for kvm with machineId: %s",
			machineId)
		return nil
	}

	kvmLogger.Debugf("running maintenance for kvm with machineId: %s", machineId)

	// Default to using the host network until we can configure.
	bridgeDevice := broker.agentConfig.Value(agent.LxcBridge)
	if bridgeDevice == "" {
		bridgeDevice = kvm.DefaultKvmBridge
	}
	_, err := configureContainerNetwork(
		machineId,
		bridgeDevice,
		broker.api,
		args.NetworkInfo,
		false, // don't allocate a new address.
		broker.enableNAT,
	)
	return err
}
