// Code shared by the CLI and API for the SetConstraints function.

package statecmd

import (
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// SetServiceContstraints sets the constraints for a given service
func SetServiceConstraints(st *state.State, args params.SetServiceConstraints) error {
	conn, err := juju.NewConnFromState(st)
	if err != nil {
		return err
	}
	var svc *state.Service
	if svc, err = conn.State.Service(args.ServiceName); err != nil {
		return err
	}
	return svc.SetConstraints(args.Constraints)
}
