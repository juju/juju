// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/juju/state"
)

// backend wraps a *state.State to implement Backend.
// It is untested, but is simple enough to be verified by inspection.
type backend struct {
	*state.State
}

func newBacked(st *state.State) Backend {
	return &backend{
		State: st,
	}
}
