// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprovisioner

import (
	"github.com/juju/juju/state"
)

// CAASProvisionerState provides the subset of global state
// required by the CAAS provisioner facade.
type CAASProvisionerState interface {
	WatchApplications() state.StringsWatcher
}
