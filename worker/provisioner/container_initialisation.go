// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"
	"sync/atomic"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/kvm"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	apiprovisioner "launchpad.net/juju-core/state/api/provisioner"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/worker"
)

// ContainerSetup is a StringsWatchHandler that is notified when containers
// are created on the given machine. It will set up the machine to be able
// to create containers and start a suitable provisioner.
type ContainerSetup struct {
	runner              worker.Runner
	supportedContainers []instance.ContainerType
	provisioner         *apiprovisioner.State
	machine             *apiprovisioner.Machine
	config              agent.Config

	// Save the workerName so the worker thread can be stopped.
	workerName string
	// setupDone[containerType] is non zero if the container setup has been invoked
	// for that container type.
	setupDone map[instance.ContainerType]*int32
	// The number of provisioners started. Once all necessary provisioners have
	// been started, the container watcher can be stopped.
	numberProvisioners int32
}

// NewContainerSetupHandler returns a StringsWatchHandler which is notified when
// containers are created on the given machine.
func NewContainerSetupHandler(runner worker.Runner, workerName string, supportedContainers []instance.ContainerType,
	machine *apiprovisioner.Machine, provisioner *apiprovisioner.State,
	config agent.Config) worker.StringsWatchHandler {

	return &ContainerSetup{
		runner:              runner,
		machine:             machine,
		supportedContainers: supportedContainers,
		provisioner:         provisioner,
		config:              config,
		workerName:          workerName,
	}
}

// SetUp is defined on the StringsWatchHandler interface.
func (cs *ContainerSetup) SetUp() (watcher watcher.StringsWatcher, err error) {
	// Set up the semaphores for each container type.
	cs.setupDone = make(map[instance.ContainerType]*int32, len(instance.ContainerTypes))
	for _, containerType := range instance.ContainerTypes {
		zero := int32(0)
		cs.setupDone[containerType] = &zero
	}
	// Listen to all container lifecycle events on our machine.
	if watcher, err = cs.machine.WatchAllContainers(); err != nil {
		return nil, err
	}
	return watcher, nil
}

// Handle is called whenever containers change on the machine being watched.
// All machines start out with so containers so the first time Handle is called,
// it will be because a container has been added.
func (cs *ContainerSetup) Handle(containerIds []string) (resultError error) {
	// Consume the initial watcher event.
	if len(containerIds) == 0 {
		return nil
	}

	logger.Tracef("initial container setup with ids: %v", containerIds)
	for _, id := range containerIds {
		containerType := state.ContainerTypeFromId(id)
		// If this container type has been dealt with, do nothing.
		if atomic.LoadInt32(cs.setupDone[containerType]) != 0 {
			continue
		}
		if err := cs.initialiseAndStartProvisioner(containerType); err != nil {
			logger.Errorf("starting container provisioner for %v: %v", containerType, err)
			// Just because dealing with one type of container fails, we won't exit the entire
			// function because we still want to try and start other container types. So we
			// take note of and return the first such error.
			if resultError == nil {
				resultError = err
			}
		}
	}
	return resultError
}

func (cs *ContainerSetup) initialiseAndStartProvisioner(containerType instance.ContainerType) error {
	// Flag that this container type has been handled.
	atomic.StoreInt32(cs.setupDone[containerType], 1)

	if atomic.AddInt32(&cs.numberProvisioners, 1) == int32(len(cs.supportedContainers)) {
		// We only care about the initial container creation.
		// This worker has done its job so stop it.
		// We do not expect there will be an error, and there's not much we can do anyway.
		if err := cs.runner.StopWorker(cs.workerName); err != nil {
			logger.Warningf("stopping machine agent container watcher: %v", err)
		}
	}

	// We only care about the initial container creation.
	// This worker has done its job so stop it.
	// We do not expect there will be an error, and there's not much we can do anyway.
	if err := cs.runner.StopWorker(cs.workerName); err != nil {
		logger.Warningf("stopping machine agent container watcher: %v", err)
	}
	if initialiser, broker, err := cs.getContainerArtifacts(containerType); err != nil {
		return fmt.Errorf("initialising container infrastructure on host machine: %v", err)
	} else {
		if err := initialiser.Initialise(); err != nil {
			return fmt.Errorf("setting up container dependnecies on host machine: %v", err)
		}
		return StartProvisioner(cs.runner, containerType, cs.provisioner, cs.config, broker)
	}
}

// TearDown is defined on the StringsWatchHandler interface.
func (cs *ContainerSetup) TearDown() error {
	// Nothing to do here.
	return nil
}

func (cs *ContainerSetup) getContainerArtifacts(containerType instance.ContainerType) (container.Initialiser, environs.InstanceBroker, error) {
	tools, err := cs.provisioner.Tools(cs.config.Tag())
	if err != nil {
		logger.Errorf("cannot get tools from machine for %s container", containerType)
		return nil, nil, err
	}
	var initialiser container.Initialiser
	var broker environs.InstanceBroker
	switch containerType {
	case instance.LXC:
		initialiser = lxc.NewContainerInitialiser()
		broker = NewLxcBroker(cs.provisioner, tools, cs.config)
	case instance.KVM:
		initialiser = kvm.NewContainerInitialiser()
		broker, err = NewKvmBroker(cs.provisioner, tools, cs.config)
		if err != nil {
			logger.Errorf("failed to create new kvm broker")
			return nil, nil, err
		}
	default:
		return nil, nil, fmt.Errorf("unknown container type: %v", containerType)
	}
	return initialiser, broker, nil
}

// Override for testing.
var StartProvisioner = startProvisionerWorker

// startProvisionerWorker kicks off a provisioner task responsible for creating containers
// of the specified type on the machine.
func startProvisionerWorker(runner worker.Runner, containerType instance.ContainerType,
	provisioner *apiprovisioner.State, config agent.Config, broker environs.InstanceBroker) error {

	workerName := fmt.Sprintf("%s-provisioner", containerType)
	// The provisioner task is created after a container record has already been added to the machine.
	// It will see that the container does not have an instance yet and create one.
	return runner.StartWorker(workerName, func() (worker.Worker, error) {
		return NewContainerProvisioner(containerType, provisioner, config, broker), nil
	})
}
