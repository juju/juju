// Code shared by the CLI and API for the DestroyServiceUnits function.

package statecmd

import (
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// DestroyServiceUnits removes the specified units.
func DestroyServiceUnits(state *state.State, args params.DestroyServiceUnits) error {
	conn, err := juju.NewConnFromState(state)
	if err != nil {
		return err
	}
	err = conn.DestroyUnits(args.UnitNames...)
	return err
}
