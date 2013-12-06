// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"launchpad.net/juju-core/container"
)

// StartParams is a simple parameter struct for Container.Start.
type StartParams struct {
	Series       string
	Arch         string
	UserDataFile string
	Network      *container.NetworkConfig
	Memory       uint64 // MB
	CpuCores     uint64
	RootDisk     uint64 // GB
}

// Container represents a virtualized container instance and provides
// operations to create, maintain and destroy the container.
type Container interface {

	// Name returns the name of the container.
	Name() string

	// Start runs the container as a daemon.
	Start(params StartParams) error

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
