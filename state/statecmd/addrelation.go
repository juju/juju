// Code shared by the CLI and API for the AddRelation function.

package statecmd

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// AddRelation adds a relation between the specified endpoints.
func AddRelation(state *state.State, args params.AddRelation) error {
	eps, err := state.InferEndpoints(args.Endpoints)
	if err != nil {
		return err
	}
	_, err = state.AddRelation(eps...)
	return err
}
