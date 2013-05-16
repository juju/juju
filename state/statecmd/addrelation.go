// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Code shared by the CLI and API for the AddRelation function.

package statecmd

import (
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// AddRelation adds a relation between the specified endpoint names, and
// returns a map from service names to relation endpoints.
func AddRelation(state *state.State, args params.AddRelation) (params.AddRelationResults, error) {
	inEps, err := state.InferEndpoints(args.Endpoints)
	if err != nil {
		return params.AddRelationResults{}, err
	}
	rel, err := state.AddRelation(inEps...)
	if err != nil {
		return params.AddRelationResults{}, err
	}
	outEps := make(map[string]charm.Relation)
	for _, inEp := range inEps {
		outEp, err := rel.Endpoint(inEp.ServiceName)
		if err != nil {
			return params.AddRelationResults{}, err
		}
		outEps[inEp.ServiceName] = outEp.Relation
	}
	return params.AddRelationResults{Endpoints: outEps}, nil
}
