// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/juju/state"
	"gopkg.in/juju/charm.v6"
)

type InstanceMutaterState interface {
	state.EntityFinder

	WatchModelMachines() state.StringsWatcher
	Unit(string) (Unit, error)
	Model() (Model, error)
}

type Model interface {
	Name() string
}

type Machine interface {
	CharmProfiles() ([]string, error)
}

type Unit interface {
	Application() (Application, error)
}

type Application interface {
	Charm() (Charm, error)
	Name() string
}

type Charm interface {
	LXDProfile() *charm.LXDProfile
	Revision() int
}
