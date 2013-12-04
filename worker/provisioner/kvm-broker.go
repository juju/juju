// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/loggo"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/kvm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/tools"
)

var kvmLogger = loggo.GetLogger("juju.provisioner.kvm")

var _ environs.InstanceBroker = (*kvmBroker)(nil)
var _ tools.HasTools = (*kvmBroker)(nil)

func NewKvmBroker(
	api APICalls,
	tools *tools.Tools,
	agentConfig agent.Config,
) (environs.InstanceBroker, error) {
	manager, err := kvm.NewContainerManager(container.ManagerConfig{Name: "juju"})
	if err != nil {
		return nil, err
	}
	return &kvmBroker{
		manager:     manager,
		api:         api,
		tools:       tools,
		agentConfig: agentConfig,
	}, nil
}

type kvmBroker struct {
	manager     container.Manager
	api         APICalls
	tools       *tools.Tools
	agentConfig agent.Config
}

func (broker *kvmBroker) Tools() tools.List {
	return tools.List{broker.tools}
}

// StartInstance is specified in the Broker interface.
func (broker *kvmBroker) StartInstance(
	cons constraints.Value,
	possibleTools tools.List,
	machineConfig *cloudinit.MachineConfig,
) (instance.Instance, *instance.HardwareCharacteristics, error) {

	// TODO: refactor common code out of the container brokers.
	machineId := machineConfig.MachineId
	kvmLogger.Infof("starting kvm container for machineId: %s", machineId)

	// TODO: Default to using the host network until we can configure.  Yes,
	// this is using the LxcBridge value, we should put it in the api call for
	// container config.
	bridgeDevice := broker.agentConfig.Value(agent.LxcBridge)
	if bridgeDevice == "" {
		bridgeDevice = kvm.DefaultKvmBridge
	}
	network := container.BridgeNetworkConfig(bridgeDevice)

	// TODO: series doesn't necessarily need to be the same as the host.
	series := possibleTools.OneSeries()
	machineConfig.MachineContainerType = instance.KVM
	machineConfig.Tools = possibleTools[0]

	config, err := broker.api.ContainerConfig()
	if err != nil {
		kvmLogger.Errorf("failed to get container config: %v", err)
		return nil, nil, err
	}
	if err := environs.PopulateMachineConfig(
		machineConfig,
		config.ProviderType,
		config.AuthorizedKeys,
		config.SSLHostnameVerification,
		config.SyslogPort,
	); err != nil {
		kvmLogger.Errorf("failed to populate machine config: %v", err)
		return nil, nil, err
	}

	inst, hardware, err := broker.manager.StartContainer(machineConfig, series, network)
	if err != nil {
		kvmLogger.Errorf("failed to start container: %v", err)
		return nil, nil, err
	}
	kvmLogger.Infof("started kvm container for machineId: %s, %s, %s", machineId, inst.Id(), hardware.String())
	return inst, hardware, nil
}

// StopInstances shuts down the given instances.
func (broker *kvmBroker) StopInstances(instances []instance.Instance) error {
	// TODO: potentially parallelise.
	for _, instance := range instances {
		kvmLogger.Infof("stopping kvm container for instance: %s", instance.Id())
		if err := broker.manager.StopContainer(instance); err != nil {
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
