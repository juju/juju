// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/kvm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/network"
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
	managerConfig container.ManagerConfig,
) (environs.InstanceBroker, error) {
	manager, err := kvm.NewContainerManager(managerConfig)
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

func (broker *kvmBroker) Tools(series string) tools.List {
	// TODO: thumper 2014-04-08 bug 1304151
	// should use the api get get tools for the series.
	seriesTools := *broker.tools
	seriesTools.Version.Series = series
	return tools.List{&seriesTools}
}

// StartInstance is specified in the Broker interface.
func (broker *kvmBroker) StartInstance(args environs.StartInstanceParams) (instance.Instance, *instance.HardwareCharacteristics, []network.Info, error) {
	if args.MachineConfig.HasNetworks() {
		return nil, nil, nil, fmt.Errorf("starting kvm containers with networks is not supported yet.")
	}
	// TODO: refactor common code out of the container brokers.
	machineId := args.MachineConfig.MachineId
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
	series := args.Tools.OneSeries()
	args.MachineConfig.MachineContainerType = instance.KVM
	args.MachineConfig.Tools = args.Tools[0]

	config, err := broker.api.ContainerConfig()
	if err != nil {
		kvmLogger.Errorf("failed to get container config: %v", err)
		return nil, nil, nil, err
	}
	if err := environs.PopulateMachineConfig(
		args.MachineConfig,
		config.ProviderType,
		config.AuthorizedKeys,
		config.SSLHostnameVerification,
		config.Proxy,
		config.AptProxy,
	); err != nil {
		kvmLogger.Errorf("failed to populate machine config: %v", err)
		return nil, nil, nil, err
	}

	inst, hardware, err := broker.manager.CreateContainer(args.MachineConfig, series, network)
	if err != nil {
		kvmLogger.Errorf("failed to start container: %v", err)
		return nil, nil, nil, err
	}
	kvmLogger.Infof("started kvm container for machineId: %s, %s, %s", machineId, inst.Id(), hardware.String())
	return inst, hardware, nil, nil
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
