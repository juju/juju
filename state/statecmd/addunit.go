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
	if args.NumUnits > 1 && args.ToMachineSpec != "" {
		// '--num-units' and '--to' are very CLI specific, should we be
		// returning a more explicit error and having the CLI translate
		// it into the arguments being supplied? We could use a generic
		// API error here, but have the CLI pre-vet its arguments in
		// addition to this code.
		return nil, errors.New("cannot use --num-units with --to")
	}
	return conn.AddUnits(service, args.NumUnits, args.ToMachineSpec)
}
