// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statecmd

// Code shared by the CLI and API for the ServiceDestroy function.

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// ServiceDestroy destroys a given service along with all its units and relations.
func ServiceDestroy(state *state.State, args params.ServiceDestroy) error {
	svc, err := state.Service(args.ServiceName)
	if err != nil {
		return err
	}

	return svc.Destroy()
}
