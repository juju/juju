// Code shared by the CLI and API for the ServiceDestroyUnits function.

package statecmd

import (
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// ServiceDestroyUnits adds a given number of units to a service.
func ServiceDestroyUnits(state *state.State, args params.ServiceDestroyUnits) error {
	conn, err := juju.NewConnFromState(state)
	if err != nil {
		return err
	}
	err = conn.DestroyUnits(args.UnitNames...)
	return err
}
