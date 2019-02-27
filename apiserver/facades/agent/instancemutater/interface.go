// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/juju/state"
)

type InstanceMutaterState interface {
	state.EntityFinder

	WatchModelMachines() state.StringsWatcher
}
