// The statecmd package is a temporary package
// to put code that's used by both cmd/juju and state/api.
package statecmd

import (
	"launchpad.net/juju-core/state"
)

// ServiceUnexposeParams stores parameters for making the ServiceUnexpose call.
type ServiceUnexposeParams struct {
	ServiceName string
}

// ServiceUnexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.  It returns an error or nil.
func ServiceUnexpose(state *state.State, args ServiceUnexposeParams) error {
	svc, err := state.Service(args.ServiceName)
	if err != nil {
		return err
	}
	return svc.ClearExposed()
}
