// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"os"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/tools"
)

var lxcLogger = loggo.GetLogger("juju.provisioner.lxc")

var _ environs.InstanceBroker = (*lxcBroker)(nil)
var _ tools.HasTools = (*lxcBroker)(nil)

func NewLxcBroker(config *config.Config, tools *tools.Tools) environs.InstanceBroker {
	return &lxcBroker{
		manager: lxc.NewContainerManager(lxc.ManagerConfig{Name: "juju"}),
		config:  config,
		tools:   tools,
	}
}

type lxcBroker struct {
	manager lxc.ContainerManager
	config  *config.Config
	tools   *tools.Tools
}

func (broker *lxcBroker) Tools() tools.List {
	return tools.List{broker.tools}
}

// StartInstance is specified in the Broker interface.
func (broker *lxcBroker) StartInstance(cons constraints.Value, possibleTools tools.List,
	machineConfig *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, error) {

	machineId := machineConfig.MachineId
	lxcLogger.Infof("starting lxc container for machineId: %s", machineId)

	// Default to using the host network until we can configure.
	bridgeDevice := os.Getenv(osenv.JujuLxcBridge)
	if bridgeDevice == "" {
		bridgeDevice = lxc.DefaultLxcBridge
	}
	network := lxc.BridgeNetworkConfig(bridgeDevice)

	series := possibleTools.OneSeries()
	inst, err := broker.manager.StartContainer(
		machineId, series, machineConfig.MachineNonce, network, possibleTools[0], broker.config,
		machineConfig.StateInfo, machineConfig.APIInfo)
	if err != nil {
		lxcLogger.Errorf("failed to start container: %v", err)
		return nil, nil, err
	}
	lxcLogger.Infof("started lxc container for machineId: %s, %s", machineId, inst.Id())
	return inst, nil, nil
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
