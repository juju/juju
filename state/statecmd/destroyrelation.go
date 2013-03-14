// Code shared by the CLI and API for the DestroyRelation function.

package statecmd

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// DestroyRelation removes the relation between the specified endpoints.
func DestroyRelation(state *state.State, args params.DestroyRelation) error {
	endpoints := []string{args.Endpoint0, args.Endpoint1}
	eps, err := state.InferEndpoints(endpoints)
	if err != nil {
		return err
	}
	rel, err := state.EndpointsRelation(eps...)
	if err != nil {
		return err
	}
	return rel.Destroy()
}
