// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
)

// ManagerConfig contains the initialization parameters for the ContainerManager.
// The name of the manager is used to namespace the containers on the machine.
type ManagerConfig struct {
	Name   string
	LogDir string
}

// Manager is responsible for starting containers, and stopping and listing
// containers that it has started.
type Manager interface {
	// StartContainer creates and starts a new lxc container for the specified machine.
	StartContainer(
		machineConfig *cloudinit.MachineConfig,
		series string,
		network *NetworkConfig) (instance.Instance, error)
	// StopContainer stops and destroyes the lxc container identified by Instance.
	StopContainer(instance.Instance) error
	// ListContainers return a list of containers that have been started by
	// this manager.
	ListContainers() ([]instance.Instance, error)
}

// Container represents a virtualized container instance and provides
// operations to create, maintain and destroy the container.
type Container interface {

	// Name returns the name of the container.
	Name() string

	// Start runs the container as a daemon.
	// TODO: determine parameters
	Start() error

	// Stop terminates the running container.
	Stop() error

	// IsRunning returns wheter or not the container is running and active.
	IsRunning() bool

	// String returns information about the container, like the name, state,
	// and process id.
	String() string
}

// ContainerFactory represents the methods used to create Containers.  This
// wraps the low level OS functions for dealing with the containers.
type ContainerFactory interface {
	// New returns a container instance which can then be used for operations
	// like Start() and Stop()
	New(string) Container

	// List returns all the existing containers on the system.
	List() ([]Container, error)
}
