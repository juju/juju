// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
)

// CAASOperatorProvisionerState provides the subset of global state
// required by the CAAS operator provisioner facade.
type CAASOperatorProvisionerState interface {
	WatchApplications() state.StringsWatcher
	FindEntity(tag names.Tag) (state.Entity, error)
}
