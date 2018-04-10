// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import "gopkg.in/juju/charm.v6"

// LocalState is a cache of the state of the operator
// It is generally compared to the remote state of the
// the application as stored in the controller.
type LocalState struct {
	// CharmModifiedVersion increases any time the charm,
	// or any part of it, is changed in some way.
	CharmModifiedVersion int

	// CharmURL reports the currently installed charm URL. This is set
	// by the committing of deploy (install/upgrade) ops.
	CharmURL *charm.URL
}
