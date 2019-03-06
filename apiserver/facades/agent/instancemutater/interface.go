// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/state"
)

type InstanceMutaterState interface {
	state.EntityFinder

	WatchModelMachines() state.StringsWatcher
	Unit(string) (Unit, error)
	Model() (Model, error)
}

// State represents point of use methods from the state object
type State interface {
	Model() (*state.Model, error)
	Unit(name string) (*state.Unit, error)
}

type Model interface {
	Name() string
}

// Machine represents point of use methods from the state machine object
type Machine interface {
	CharmProfiles() ([]string, error)
	InstanceId() (instance.Id, error)
	SetCharmProfiles([]string) error
	SetUpgradeCharmProfileComplete(unitName, msg string) error
}

// Unit represents point of use methods from the state unit object
type Unit interface {
	Application() (Application, error)
}

// Application represents point of use methods from the state application object
type Application interface {
	Charm() (Charm, error)
	Name() string
}

// Charm represents point of use methods from the state charm object
type Charm interface {
	LXDProfile() LXDProfile
	Revision() int
}

type LXDProfile interface {
	Config() map[string]string
	Description() string
	Devices() map[string]map[string]string
	Empty() bool
	ValidateConfigDevices() error
}
