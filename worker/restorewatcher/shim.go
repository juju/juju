// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package restorewatcher

import "github.com/juju/juju/v2/state"

type RestoreInfoWatcherShim struct {
	*state.State
}

func (r RestoreInfoWatcherShim) RestoreStatus() (state.RestoreStatus, error) {
	return r.RestoreInfo().Status()
}
