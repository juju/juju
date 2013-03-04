// Code shared by the CLI and API for the ServiceAddUnit function.

package statecmd

import (
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
)

type ServiceAddUnitParams struct {
	ServiceName string
	NumUnits int
}

func ServiceAddUnit(state *state.State, args ServiceAddUnitParams) error {
	conn, err := juju.NewConnFromState(state)
	if err != nil {
		return err
	}
	service, err := state.Service(args.ServiceName)
	if err != nil {
		return err
	}
	_, err = service.AddUnits(service, args.NumUnits)
	return err
}
