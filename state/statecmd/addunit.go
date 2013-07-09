// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Code shared by the CLI and API for the ServiceAddUnit function.

package statecmd

import (
	"errors"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// AddServiceUnits adds a given number of units to a service.
func AddServiceUnits(state *state.State, args params.AddServiceUnits) ([]*state.Unit, error) {
	conn, err := juju.NewConnFromState(state)
	if err != nil {
		return nil, err
	}
	service, err := state.Service(args.ServiceName)
	if err != nil {
		return nil, err
	}
	if args.NumUnits < 1 {
		return nil, errors.New("must add at least one unit")
	}
	return conn.AddUnits(service, args.NumUnits, "", "")
}
