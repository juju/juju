// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import "github.com/juju/juju/state"

type upgradeStepsStateShim struct {
	*state.State
}
