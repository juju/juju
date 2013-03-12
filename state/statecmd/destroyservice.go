package statecmd

// Code shared by the CLI and API for the ServiceDestroy function.

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// ServiceDestroy changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func ServiceDestroy(state *state.State, args params.ServiceDestroy) error {
	svc, err := state.Service(args.ServiceName)
	if err != nil {
		return err
	}

	return svc.Destroy()
}
