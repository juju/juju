// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"time"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

// InstanceMutaterState represents point of use methods from the state object.
type InstanceMutaterState interface {
	state.EntityFinder

	Application(appName string) (Application, error)
	Machine(id string) (Machine, error)
	Unit(unitName string) (Unit, error)
	ControllerTimestamp() (*time.Time, error)

	WatchMachines() state.StringsWatcher
	WatchModelMachines() state.StringsWatcher
	WatchApplicationCharms() state.StringsWatcher
	WatchUnits() state.StringsWatcher
}

// Machine represents point of use methods from the state Machine object.
type Machine interface {
	Id() string
	ContainerType() instance.ContainerType
	IsManual() (bool, error)
	SetModificationStatus(status.StatusInfo) error
	Units() ([]Unit, error)
	WatchContainers(instance.ContainerType) state.StringsWatcher
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

// Application represents point of use methods from the state Application object.
type Application interface {
	CharmURL() *string
}
