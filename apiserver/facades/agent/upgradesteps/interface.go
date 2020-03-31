// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

type UpgradeStepsState interface {
	state.EntityFinder
}

// Machine represents point of use methods from the state machine object
type Machine interface {
	ContainerType() instance.ContainerType
	ModificationStatus() (status.StatusInfo, error)
	SetModificationStatus(status.StatusInfo) error
}

// Unit represents point of use methods from the state unit object
type Unit interface {
	SetState(*state.UnitState) error
}
