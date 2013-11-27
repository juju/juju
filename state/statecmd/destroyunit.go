// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Code shared by the CLI and API for the DestroyServiceUnits function.

package statecmd

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// DestroyServiceUnits removes the specified units.
func DestroyServiceUnits(st *state.State, args params.DestroyServiceUnits) error {
	return st.DestroyUnits(args.UnitNames...)
}
