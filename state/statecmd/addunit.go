// Code shared by the CLI and API for the ServiceAddUnit function.

package statecmd

import (
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

func ServiceAddUnit(state *state.State, args params.ServiceAddUnit) error {
	conn, err := juju.NewConnFromState(state)
	if err != nil {
		return err
	}
	service, err := state.Service(args.ServiceName)
	if err != nil {
		return err
	}
	_, err = conn.AddUnits(service, args.NumUnits)
	return err
}
