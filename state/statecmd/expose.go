// Code shared by the CLI and API for the ServiceExpose function.

package statecmd

import (
	"launchpad.net/juju-core/state"
)

// ServiceExposeParams are parameters for making the ServiceExpose call.
type ServiceExposeParams struct {
	ServiceName string
}

// ServiceExpose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.  It returns any errors or
// nil.
func ServiceExpose(state *state.State, args ServiceExposeParams) error {
	svc, err := state.Service(args.ServiceName)
	if err != nil {
		return err
	}

	return svc.SetExposed()
}
