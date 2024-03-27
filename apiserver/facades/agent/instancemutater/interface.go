// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"time"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

// InstanceMutaterState represents point of use methods from the state object.
type InstanceMutaterState interface {
	state.EntityFinder

	ModelName() (string, error)
	Application(appName string) (Application, error)
	Charm(curl string) (Charm, error)
	Machine(id string) (Machine, error)
	Unit(unitName string) (Unit, error)
	ControllerTimestamp() (*time.Time, error)

	WatchMachines() state.StringsWatcher
	WatchModelMachines() state.StringsWatcher
	WatchApplicationCharms() state.StringsWatcher
	WatchCharms() state.StringsWatcher
	WatchUnits() state.StringsWatcher
}

// Machine represents point of use methods from the state Machine object.
type Machine interface {
	Id() string
	InstanceId() (instance.Id, error)
	ContainerType() instance.ContainerType
	IsManual() (bool, error)
	CharmProfiles() ([]string, error)
	SetCharmProfiles([]string) error
	SetModificationStatus(status.StatusInfo, status.StatusHistoryRecorder) error
	Units() ([]Unit, error)
	WatchContainers(instance.ContainerType) state.StringsWatcher
	WatchInstanceData() state.NotifyWatcher
}

// Unit represents a point of use methods from the state Unit object.
type Unit interface {
	Name() string
	Life() state.Life
	ApplicationName() string
	Application() (Application, error)
	PrincipalName() (string, bool)
	AssignedMachineId() (string, error)
	CharmURL() *string
}

// Charm represents point of use methods from the state Charm object.
type Charm interface {
	LXDProfile() lxdprofile.Profile
	Revision() int
}

// Application represents point of use methods from the state Application object.
type Application interface {
	CharmURL() *string
}
