// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import "github.com/juju/charm/v7"

// Snapshot is a snapshot of the remote state of the unit.
type Snapshot struct {
	// CharmModifiedVersion is increased whenever the application's charm was
	// changed in some way.
	CharmModifiedVersion int

	// CharmURL is the charm URL that the unit is
	// expected to run.
	CharmURL *charm.URL

	// ForceCharmUpgrade reports whether the unit
	// should upgrade even in an error state.
	ForceCharmUpgrade bool
}
