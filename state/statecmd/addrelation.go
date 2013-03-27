// Code shared by the CLI and API for the AddRelation function.

package statecmd

import (
	"fmt"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// AddRelation adds a relation between the specified endpoints.
func AddRelation(state *state.State, args params.AddRelation) (params.AddRelationResults, error) {
	if len(args.Endpoints) != 2 {
		return params.AddRelationResults{}, fmt.Errorf("a relation must involve two services")
	}
	eps, err := state.InferEndpoints(args.Endpoints)
	if err != nil {
		return params.AddRelationResults{}, err
	}
	_, err = state.AddRelation(eps...)
	if err != nil {
		return params.AddRelationResults{}, err
	}
	relInfo := params.AddRelationResults{}
	relInfo.Endpoints = []string{}
	relInfo.Interface = ""
	relInfo.Scope = ""
	return relInfo, nil
}
