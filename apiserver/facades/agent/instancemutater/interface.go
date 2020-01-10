// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"time"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

type InstanceMutaterState interface {
	state.EntityFinder

	ControllerTimestamp() (*time.Time, error)
}

// Machine represents point of use methods from the state machine object
type Machine interface {
	InstanceId() (instance.Id, error)
	CharmProfiles() []string
	Units() ([]ModelCacheUnit, error)
	SetCharmProfiles([]string) error
	SetModificationStatus(status.StatusInfo) error
}

type LXDProfile interface {
	Config() map[string]string
	Description() string
	Devices() map[string]map[string]string
	Empty() bool
	ValidateConfigDevices() error
}

// ModelCache represents point of use methods from the cache
// model
type ModelCache interface {
	Name() string
	Application(appName string) (ModelCacheApplication, error)
	Charm(charmURL string) (ModelCacheCharm, error)
	Machine(machineId string) (ModelCacheMachine, error)
	WatchMachines() (cache.StringsWatcher, error)
}

// ModelCacheApplication represents a point of use Application from the cache
// package.
type ModelCacheApplication interface {
	CharmURL() string
}

// ModelCacheMachine represents a point of use Machine from the cache package.
type ModelCacheMachine interface {
	InstanceId() (instance.Id, error)
	CharmProfiles() []string
	ContainerType() instance.ContainerType
	WatchLXDProfileVerificationNeeded() (cache.NotifyWatcher, error)
	WatchContainers() (cache.StringsWatcher, error)
	Units() ([]ModelCacheUnit, error)
}

// ModelCacheUnit represents a point of use Unit from the cache package.
type ModelCacheUnit interface {
	Application() string
}

// ModelCacheCharm represents point of use methods from the cache charm object
type ModelCacheCharm interface {
	LXDProfile() lxdprofile.Profile
}
