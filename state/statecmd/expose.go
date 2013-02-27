// The statecmd package is a temporary package
// to put code that's used by both cmd/juju and state/api.
// It is intended to wither away to nothing as functionality
// gets absorbed into state and state/api as appropriate
// when the command-line commands can invoke the
// API directly.
package statecmd

import (
	"launchpad.net/juju-core/state"
)

// Parameters for making the ServiceExpose call.
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
