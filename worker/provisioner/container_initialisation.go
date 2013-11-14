// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"
	"sync/atomic"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/instance"
	apiprovisioner "launchpad.net/juju-core/state/api/provisioner"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/worker"
)

// ContainerSetup is a StringsWatchHandler that is notified when containers of
// the specified type are created on the given machine. It will set up the
// machine to be able to create containers and start a provisioner.
type ContainerSetup struct {
	runner        worker.Runner
	containerType instance.ContainerType
	provisioner   *apiprovisioner.State
	machine       *apiprovisioner.Machine
	config        agent.Config

	// Save the workerName so the worker thread can be stopped.
	workerName string
	// setupDone is non zero if the container setup has been invoked.
	setupDone int32
}

// NewContainerSetupHandler returns a StringsWatchHandler which is notified when
// containers are created on the given machine.
func NewContainerSetupHandler(runner worker.Runner, workerName string, container instance.ContainerType,
	machine *apiprovisioner.Machine, provisioner *apiprovisioner.State,
	config agent.Config) worker.StringsWatchHandler {

	return &ContainerSetup{
		runner:        runner,
		containerType: container,
		machine:       machine,
		provisioner:   provisioner,
		config:        config,
		workerName:    workerName,
	}
}

// SetUp is defined on the StringsWatchHandler interface.
func (cs *ContainerSetup) SetUp() (watcher watcher.StringsWatcher, err error) {
	if watcher, err = cs.machine.WatchContainers(cs.containerType); err != nil {
		return nil, err
	}
	return watcher, nil
}

// Handle is called whenever containers change on the machine being watched.
// All machines start out with so containers so the first time Handle is called,
// it will be because a container has been added.
func (cs *ContainerSetup) Handle(containerIds []string) error {
	// Consume the initial watcher event.
	if len(containerIds) == 0 {
		return nil
	}

	// This callback must only be invoked once. Stopping the watcher
	// below should be sufficient but I'm paranoid.
	if atomic.LoadInt32(&cs.setupDone) != 0 {
		return nil
	}
	atomic.StoreInt32(&cs.setupDone, 1)

	logger.Tracef("initial container setup with ids: %v", containerIds)
	// We only care about the initial container creation.
	// This worker has done its job so stop it.
	cs.runner.StopWorker(cs.workerName)
	if err := cs.ensureContainerDependencies(); err != nil {
		return fmt.Errorf("setting up container dependnecies on host machine: %v", err)
	}
	var provisionerType ProvisionerType
	switch cs.containerType {
	case instance.LXC:
		provisionerType = LXC
	case instance.KVM:
		provisionerType = KVM
	default:
		return fmt.Errorf("invalid container type %q", cs.containerType)
	}
	return StartProvisioner(cs.runner, provisionerType, cs.provisioner, cs.config)
}

// TearDown is defined on the StringsWatchHandler interface.
func (cs *ContainerSetup) TearDown() error {
	// Nothing to do here.
	return nil
}

func (cs *ContainerSetup) ensureContainerDependencies() error {
	// TODO(wallyworld) - install whatever dependencies are required to support starting containers
	return nil
}

// Override for testing.
var StartProvisioner = startProvisionerWorker

// startProvisionerWorker kicks off a provisioner task responsible for creating containers
// of the specified type on the machine.
func startProvisionerWorker(runner worker.Runner, provisionerType ProvisionerType,
	provisioner *apiprovisioner.State, config agent.Config) error {

	workerName := fmt.Sprintf("%s-provisioner", provisionerType)
	// The provisioner task is created after a container record has already been added to the machine.
	// It will see that the container does not have an instance yet and create one.
	return runner.StartWorker(workerName, func() (worker.Worker, error) {
		return NewProvisioner(provisionerType, provisioner, config), nil
	})
}
