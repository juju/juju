// Code shared by the CLI and API for the ServiceAddUnit function.

package statecmd

import (
	"errors"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// ServiceAddUnits adds a given number of units to a service.
func ServiceAddUnits(state *state.State, args params.ServiceAddUnits) error {
	conn, err := juju.NewConnFromState(state)
	if err != nil {
		return err
	}
	service, err := state.Service(args.ServiceName)
	if err != nil {
		return err
	}
	if args.NumUnits < 1 {
		return errors.New("must add at least one unit")
	}
	_, err = conn.AddUnits(service, args.NumUnits)
	return err
}
