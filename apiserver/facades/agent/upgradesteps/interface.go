// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"context"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

// ControllerConfigGetter is an interface that provides access to the
// controller configuration.
type ControllerConfigGetter interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// UpgradeStepsState represents the state methods used by the upgrade steps
// facade.
type UpgradeStepsState interface {
	state.EntityFinder
	ApplyOperation(state.ModelOperation) error
}

// Machine represents point of use methods from the state machine object
type Machine interface {
	ContainerType() instance.ContainerType
	ModificationStatus() (status.StatusInfo, error)
	SetModificationStatus(status.StatusInfo, status.StatusHistoryRecorder) error
}

// Unit represents point of use methods from the state unit object
type Unit interface {
	SetStateOperation(*state.UnitState, state.UnitStateSizeLimits) state.ModelOperation
}

var (
	// NOTE(achilleasa): If the above interface definitions are not
	// compatible to the equivalent implementations in state, upgrades can
	// break due to failed cast checks. The following compile-time checks
	// allow us to catch such issues.
	_ Unit    = (*state.Unit)(nil)
	_ Machine = (*state.Machine)(nil)
)
