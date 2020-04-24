// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"github.com/juju/charm/v7"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
)

const (
	ConfigModelUUID        = "model-uuid"
	ConfigLogDir           = "log-dir"
	ConfigAvailabilityZone = "availability-zone"
)

//go:generate go run github.com/golang/mock/mockgen -package testing -destination testing/package_mock.go github.com/juju/juju/container Manager,Initialiser

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
		cons constraints.Value,
		series string,
		network *NetworkConfig,
		storage *StorageConfig,
		callback environs.StatusCallbackFunc) (instances.Instance, *instance.HardwareCharacteristics, error)

	// DestroyContainer stops and destroys the container identified by
	// instance id.
	DestroyContainer(instance.Id) error

	// ListContainers return a list of containers that have been started by
	// this manager.
	ListContainers() ([]instances.Instance, error)

	// IsInitialized check whether or not required packages have been installed
	// to support this manager.
	IsInitialized() bool

	// Namespace returns the namespace of the manager. This namespace defines the
	// prefix of all the hostnames.
	Namespace() instance.Namespace
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
		logger.Infof("unused config option: %q -> %q", key, value)
	}
}

// LXDProfileManager defines an interface for dealing with lxd profiles used to
// deploy juju containers.
type LXDProfileManager interface {
	// AssignLXDProfiles assigns the given profile names to the lxd instance
	// provided.  The slice of ProfilePosts provides details for adding to
	// and removing profiles from the lxd server.
	AssignLXDProfiles(instId string, profilesNames []string, profilePosts []lxdprofile.ProfilePost) ([]string, error)

	// MaybeWriteLXDProfile, write given LXDProfile to machine if not already
	// there.
	MaybeWriteLXDProfile(pName string, put *charm.LXDProfile) error
}

// LXDProfileNameRetriever defines an interface for dealing with lxd profile
// names used to deploy juju containers.
type LXDProfileNameRetriever interface {
	// LXDProfileNames returns the list of available LXDProfile names from the
	// manager.
	LXDProfileNames(string) ([]string, error)
}
