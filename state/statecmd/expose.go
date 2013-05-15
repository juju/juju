// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Code shared by the CLI and API for the ServiceExpose function.

package statecmd

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// ServiceExpose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func ServiceExpose(state *state.State, args params.ServiceExpose) error {
	svc, err := state.Service(args.ServiceName)
	if err != nil {
		return err
	}

	return svc.SetExposed()
}
