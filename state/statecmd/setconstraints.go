// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Code shared by the CLI and API for the SetConstraints function.

package statecmd

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// SetServiceContstraints sets the constraints for a given service
func SetServiceConstraints(st *state.State, args params.SetServiceConstraints) error {
	svc, err := st.Service(args.ServiceName)
	if err != nil {
		return err
	}
	return svc.SetConstraints(args.Constraints)
}
