// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
)

const (
	ConfigName   = "name"
	ConfigLogDir = "log-dir"
)

// ManagerConfig contains the initialization parameters for the ContainerManager.
// The name of the manager is used to namespace the containers on the machine.
type ManagerConfig map[string]string

// Manager is responsible for starting containers, and stopping and listing
// containers that it has started.
type Manager interface {
	// CreateContainer creates and starts a new container for the specified
	// machine.
	CreateContainer(
		machineConfig *cloudinit.MachineConfig,
		series string,
		network *NetworkConfig) (instance.Instance, *instance.HardwareCharacteristics, error)

	// DestroyContainer stops and destroyes the container identified by
	// instance id.
	DestroyContainer(instance.Id) error

	// ListContainers return a list of containers that have been started by
	// this manager.
	ListContainers() ([]instance.Instance, error)
}

// Initialiser is responsible for performing the steps required to initialise
// a host machine so it can run containers.
type Initialiser interface {
	// Initialise installs all required packages, sync any images etc so
	// that the host machine can run containers.
	Initialise() error
}

// PopValue returns the requested key from the config map. If the value
// doesn't exist, the function returns the empty string. If the value does
// exist, the value is returned, and the element removed from the map.
func (m ManagerConfig) PopValue(key string) string {
	value := m[key]
	delete(m, key)
	return value
}

// WarnAboutUnused emits a warning about each value in the map.
func (m ManagerConfig) WarnAboutUnused() {
	for key, value := range m {
		logger.Warningf("unused config option: %q -> %q", key, value)
	}
}
