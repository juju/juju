// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"time"

	"github.com/juju/charm/v7"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

// InstanceMutaterState represents point of use methods from the state object.
type InstanceMutaterState interface {
	state.EntityFinder

	Application(appName string) (Application, error)
	Charm(curl *charm.URL) (Charm, error)
	ControllerTimestamp() (*time.Time, error)
}

// Machine represents point of use methods from the state Machine object.
type Machine interface {
	InstanceId() (instance.Id, error)
	CharmProfiles() ([]string, error)
	SetCharmProfiles([]string) error
	SetModificationStatus(status.StatusInfo) error
	Units() ([]Unit, error)
}

// Unit represents a point of use methods from the state Unit object.
type Unit interface {
	Application() string
}

// Charm represents point of use methods from the state Charm object.
type Charm interface {
	LXDProfile() lxdprofile.Profile
}

// Application represents point of use methods from the state Application object.
type Application interface {
	CharmURL() *charm.URL
}

// ModelCache represents point of use methods from the cache
// model
type ModelCache interface {
	Name() string
	Machine(machineId string) (ModelCacheMachine, error)
	WatchMachines() (cache.StringsWatcher, error)
}

// ModelCacheMachine represents a point of use Machine from the cache package.
type ModelCacheMachine interface {
	ContainerType() instance.ContainerType
	WatchLXDProfileVerificationNeeded() (cache.NotifyWatcher, error)
	WatchContainers() (cache.StringsWatcher, error)
}
