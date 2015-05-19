// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/instance"
)

const (
	ConfigName   = "name"
	ConfigLogDir = "log-dir"

	// ConfigIPForwarding, if set to a non-empty value, instructs the
	// container manager to enable IP forwarding as part of the
	// container initialization. Will be enabled if the enviroment
	// supports networking.
	ConfigIPForwarding = "ip-forwarding"

	// ConfigEnableNAT, if set to a non-empty value, instructs the
	// container manager to enable NAT for hosted containers. NAT is
	// required for AWS, but should be disabled for MAAS.
	ConfigEnableNAT = "enable-nat"

	// ConfigLXCDefaultMTU, if set to a positive integer (serialized
	// as a string), will cause all network interfaces on all created
	// LXC containers (not KVM instances) to use the given MTU
	// setting.
	ConfigLXCDefaultMTU = "lxc-default-mtu"

	DefaultNamespace = "juju"
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
		instanceConfig *instancecfg.InstanceConfig,
		series string,
		network *NetworkConfig,
		storage *StorageConfig) (instance.Instance, *instance.HardwareCharacteristics, error)

	// DestroyContainer stops and destroyes the container identified by
	// instance id.
	DestroyContainer(instance.Id) error

	// ListContainers return a list of containers that have been started by
	// this manager.
	ListContainers() ([]instance.Instance, error)

	// IsInitialized check whether or not required packages have been installed
	// to support this manager.
	IsInitialized() bool
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
