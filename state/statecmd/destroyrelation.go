// Code shared by the CLI and API for the DestroyRelation function.

package statecmd

import (
	"fmt"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// DestroyRelation removes the relation between the specified endpoints.
func DestroyRelation(state *state.State, args params.DestroyRelation) error {
	if len(args.Endpoints) != 2 {
		return fmt.Errorf("a relation must involve two services")
	}
	eps, err := state.InferEndpoints(args.Endpoints)
	if err != nil {
		return err
	}
	rel, err := state.EndpointsRelation(eps...)
	if err != nil {
		return err
	}
	return rel.Destroy()
}
