// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/utils/clock"
)

// GetClock exists to allow us to patch out time-handling; specifically
// for the worker/uniter tests that want to know what happens when leases
// expire unexpectedly.
//
// TODO(fwereade): lp:1479653
// This is *clearly* a bad idea, and we should be injecting the dependency
// explicitly -- and using an injected clock across the codebase -- but,
// time pressure.
var GetClock = func() clock.Clock {
	return clock.WallClock
}
