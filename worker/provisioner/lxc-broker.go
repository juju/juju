// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/loggo"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/tools"
)

var lxcLogger = loggo.GetLogger("juju.provisioner.lxc")

var _ environs.InstanceBroker = (*lxcBroker)(nil)
var _ tools.HasTools = (*lxcBroker)(nil)

type APICalls interface {
	ContainerConfig() (params.ContainerConfig, error)
}

func NewLxcBroker(api APICalls, tools *tools.Tools, agentConfig agent.Config) environs.InstanceBroker {
	return &lxcBroker{
		manager:     lxc.NewContainerManager(container.ManagerConfig{Name: "juju"}),
		api:         api,
		tools:       tools,
		agentConfig: agentConfig,
	}
}

type lxcBroker struct {
	manager     container.Manager
	api         APICalls
	tools       *tools.Tools
	agentConfig agent.Config
}

func (broker *lxcBroker) Tools() tools.List {
	return tools.List{broker.tools}
}

// StartInstance is specified in the Broker interface.
func (broker *lxcBroker) StartInstance(cons constraints.Value, possibleTools tools.List,
	machineConfig *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, error) {
	// TODO: refactor common code out of the container brokers.
	machineId := machineConfig.MachineId
	lxcLogger.Infof("starting lxc container for machineId: %s", machineId)

	// Default to using the host network until we can configure.
	bridgeDevice := broker.agentConfig.Value(agent.LxcBridge)
	if bridgeDevice == "" {
		bridgeDevice = lxc.DefaultLxcBridge
	}
	network := container.BridgeNetworkConfig(bridgeDevice)

	series := possibleTools.OneSeries()
	machineConfig.MachineContainerType = instance.LXC
	machineConfig.Tools = possibleTools[0]

	config, err := broker.api.ContainerConfig()
	if err != nil {
		lxcLogger.Errorf("failed to get container config: %v", err)
		return nil, nil, err
	}
	if err := environs.PopulateMachineConfig(
		machineConfig,
		config.ProviderType,
		config.AuthorizedKeys,
		config.SSLHostnameVerification,
		config.SyslogPort,
	); err != nil {
		lxcLogger.Errorf("failed to populate machine config: %v", err)
		return nil, nil, err
	}

	inst, hardware, err := broker.manager.StartContainer(machineConfig, series, network)
	if err != nil {
		lxcLogger.Errorf("failed to start container: %v", err)
		return nil, nil, err
	}
	lxcLogger.Infof("started lxc container for machineId: %s, %s, %s", machineId, inst.Id(), hardware.String())
	return inst, hardware, nil
}

// StopInstances shuts down the given instances.
func (broker *lxcBroker) StopInstances(instances []instance.Instance) error {
	// TODO: potentially parallelise.
	for _, instance := range instances {
		lxcLogger.Infof("stopping lxc container for instance: %s", instance.Id())
		if err := broker.manager.StopContainer(instance); err != nil {
			lxcLogger.Errorf("container did not stop: %v", err)
			return err
		}
	}
	return nil
}

// AllInstances only returns running containers.
func (broker *lxcBroker) AllInstances() (result []instance.Instance, err error) {
	return broker.manager.ListContainers()
}
