// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"
	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/instance"
	apiprovisioner "launchpad.net/juju-core/state/api/provisioner"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/worker"
)

type ContainerSetup struct {
	runner        worker.Runner
	workerName    string
	containerType instance.ContainerType
	provisioner   *apiprovisioner.State
	machine       *apiprovisioner.Machine
	config        agent.Config
}

func NewContainerSetupHandler(runner worker.Runner, workerName string, container instance.ContainerType,
	machine *apiprovisioner.Machine, provisioner *apiprovisioner.State,
	config agent.Config) worker.StringsWatchHandler {

	return &ContainerSetup{
		runner:        runner,
		workerName:    workerName,
		machine:       machine,
		containerType: container,
		provisioner:   provisioner,
		config:        config,
	}
}

func (cs *ContainerSetup) SetUp() (watcher.StringsWatcher, error) {
	containerWatcher, err := cs.machine.WatchContainers(cs.containerType)
	if err != nil {
		return nil, err
	}
	return containerWatcher, nil
}

func (cs *ContainerSetup) Handle(containerIds []string) error {
	logger.Tracef("initial container setup with ids: %v", containerIds)
	cs.runner.StopWorker(cs.workerName)
	if err := cs.ensureContainerDependencies(); err != nil {
		return fmt.Errorf("setting up container dependnecies on host machine: %v", err)
	}
	return cs.startProvisioner()
}

func (cs *ContainerSetup) TearDown() error {
	// Nothing to do here.
	return nil
}

func (cs *ContainerSetup) ensureContainerDependencies() error {
	// TODO
	return nil
}

func (cs *ContainerSetup) startProvisioner() error {

	// TODO - add check so that startProvisionerCallback is only ever called once, for each container type
	workerName := fmt.Sprintf("%s-provisioner", cs.containerType)
	var provisionerType ProvisionerType
	switch cs.containerType {
	case instance.LXC:
		provisionerType = LXC
	case instance.KVM:
		provisionerType = KVM
	default:
		return fmt.Errorf("invalid container type %q", cs.containerType)
	}

	return cs.runner.StartWorker(workerName, func() (worker.Worker, error) {
		return NewProvisioner(provisionerType, cs.provisioner, cs.config), nil
	})
}
