// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprovisioner

import (
	"github.com/juju/juju/state"
)

// CAASProvisionerState provides the subset of global state
// required by the CAAS provisioner facade.
type CAASProvisionerState interface {
	CAASModel() (CAASModel, error)
	WatchApplications() state.StringsWatcher
}

type stateShim struct {
	*state.State
}

type CAASModel interface {
	ConnectionConfig() (state.CAASConnectionConfig, error)
}

func (st stateShim) CAASModel() (CAASModel, error) {
	return st.State.CAASModel()
}
