// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The statecmd package is a temporary package
// to put code that's used by both cmd/juju and state/api.
package statecmd

import (
	_ "launchpad.net/juju-core/juju" // TODO(rog) remove this
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// ServiceUnexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func ServiceUnexpose(state *state.State, args params.ServiceUnexpose) error {
	svc, err := state.Service(args.ServiceName)
	if err != nil {
		return err
	}
	return svc.ClearExposed()
}
