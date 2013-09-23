// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/loggo"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container/kvm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/tools"
)

var kvmLogger = loggo.GetLogger("juju.provisioner.kvm")

var _ environs.InstanceBroker = (*kvmBroker)(nil)
var _ tools.HasTools = (*kvmBroker)(nil)

func NewKvmBroker(
	config *config.Config,
	tools *tools.Tools,
	agentConfig agent.Config,
) environs.InstanceBroker {
	return &kvmBroker{
		manager:     kvm.NewContainerManager(kvm.ManagerConfig{Name: "juju"}),
		config:      config,
		tools:       tools,
		agentConfig: agentConfig,
	}
}

type kvmBroker struct {
	manager     kvm.ContainerManager
	config      *config.Config
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

	machineId := machineConfig.MachineId
	kvmLogger.Infof("starting kvm container for machineId: %s", machineId)

	// Default to using the host network until we can configure.
	bridgeDevice := broker.agentConfig.Value(agent.KvmBridge)
	if bridgeDevice == "" {
		bridgeDevice = kvm.DefaultKvmBridge
	}
	network := kvm.BridgeNetworkConfig(bridgeDevice)
	// TODO: series doesn't necessarily need to be the same as the host.
	series := possibleTools.OneSeries()
	inst, err := broker.manager.StartContainer(
		machineId, series, machineConfig.MachineNonce, network, possibleTools[0], broker.config,
		machineConfig.StateInfo, machineConfig.APIInfo)
	if err != nil {
		kvmLogger.Errorf("failed to start container: %v", err)
		return nil, nil, err
	}
	kvmLogger.Infof("started kvm container for machineId: %s, %s", machineId, inst.Id())
	return inst, nil, nil
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
