// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api/base"
	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
)

const uniterFacade = "Uniter"

// State provides access to the Uniter API facade.
type State struct {
	*common.EnvironWatcher
	*common.APIAddresser

	caller base.Caller
	// unitTag contains the authenticated unit's tag.
	unitTag string
}

// NewState creates a new client-side Uniter facade.
func NewState(caller base.Caller, authTag string) *State {
	return &State{
		EnvironWatcher: common.NewEnvironWatcher(uniterFacade, caller),
		APIAddresser:   common.NewAPIAddresser(uniterFacade, caller),
		caller:         caller,
		unitTag:        authTag,
	}
}

func (st *State) call(method string, params, results interface{}) error {
	return st.caller.Call(uniterFacade, "", method, params, results)
}

// life requests the lifecycle of the given entity from the server.
func (st *State) life(tag string) (params.Life, error) {
	return common.Life(st.caller, uniterFacade, tag)
}

// relation requests relation information from the server.
func (st *State) relation(relationTag, unitTag string) (params.RelationResult, error) {
	nothing := params.RelationResult{}
	var result params.RelationResults
	args := params.RelationUnits{
		RelationUnits: []params.RelationUnit{
			{Relation: relationTag, Unit: unitTag},
		},
	}
	err := st.call("Relation", args, &result)
	if err != nil {
		return nothing, err
	}
	if len(result.Results) != 1 {
		return nothing, fmt.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return nothing, err
	}
	return result.Results[0], nil
}

// Unit provides access to methods of a state.Unit through the facade.
func (st *State) Unit(tag string) (*Unit, error) {
	life, err := st.life(tag)
	if err != nil {
		return nil, err
	}
	return &Unit{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}

// Service returns a service state by tag.
func (st *State) Service(tag string) (*Service, error) {
	life, err := st.life(tag)
	if err != nil {
		return nil, err
	}
	return &Service{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}

// ProviderType returns a provider type used by the current juju
// environment.
//
// TODO(dimitern): We might be able to drop this, once we have machine
// addresses implemented fully. See also LP bug 1221798.
func (st *State) ProviderType() (string, error) {
	var result params.StringResult
	err := st.call("ProviderType", nil, &result)
	if err != nil {
		return "", err
	}
	if err := result.Error; err != nil {
		return "", err
	}
	return result.Result, nil
}

// Charm returns the charm with the given URL.
func (st *State) Charm(curl *charm.URL) (*Charm, error) {
	if curl == nil {
		return nil, fmt.Errorf("charm url cannot be nil")
	}
	return &Charm{
		st:  st,
		url: curl.String(),
	}, nil
}

// Relation returns the existing relation with the given tag.
func (st *State) Relation(tag string) (*Relation, error) {
	result, err := st.relation(tag, st.unitTag)
	if err != nil {
		return nil, err
	}
	return &Relation{
		id:   result.Id,
		tag:  tag,
		life: result.Life,
		st:   st,
	}, nil
}

// RelationById returns the existing relation with the given id.
func (st *State) RelationById(id int) (*Relation, error) {
	var results params.RelationResults
	args := params.RelationIds{
		RelationIds: []int{id},
	}
	err := st.call("RelationById", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return nil, err
	}
	relationTag := names.RelationTag(result.Key)
	return &Relation{
		id:   result.Id,
		tag:  relationTag,
		life: result.Life,
		st:   st,
	}, nil
}

// Environment returns the environment entity.
func (st *State) Environment() (*Environment, error) {
	var result params.EnvironmentResult
	err := st.call("CurrentEnvironment", nil, &result)
	if params.IsCodeNotImplemented(err) {
		// Fall back to using the 1.16 API.
		return st.environment1dot16()
	}
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, err
	}
	return &Environment{
		name: result.Name,
		uuid: result.UUID,
	}, nil
}

// environment1dot16 requests just the UUID of the current environment, when
// using an older API server that does not support CurrentEnvironment API call.
func (st *State) environment1dot16() (*Environment, error) {
	var result params.StringResult
	err := st.call("CurrentEnvironUUID", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, err
	}
	return &Environment{
		uuid: result.Result,
	}, nil
}
