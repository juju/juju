// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/container"
)

// StartParams is a simple parameter struct for Container.Start.
type StartParams struct {
	Version           string
	Arch              string
	Stream            string
	UserDataFile      string
	NetworkConfigData string
	Network           *container.NetworkConfig
	Memory            uint64 // MB
	CpuCores          uint64
	RootDisk          uint64 // GB
	ImageDownloadURL  string
	StatusCallback    func(status status.Status, info string, data map[string]interface{}) error
}

// Container represents a virtualized container instance and provides
// operations to create, maintain and destroy the container.
type Container interface {

	// Name returns the name of the container.
	Name() string

	// EnsureCachedImage ensures that a container image suitable for satisfying
	// the input start parameters has been cached on disk.
	EnsureCachedImage(params StartParams) error

	// Start runs the container as a daemon.
	Start(params StartParams) error

	// Stop terminates the running container.
	Stop() error

	// IsRunning returns whether or not the container is running and active.
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
