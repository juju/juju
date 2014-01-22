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

const uniter = "Uniter"

// State provides access to the Uniter API facade.
type State struct {
	*common.EnvironWatcher

	caller base.Caller
	// unitTag contains the authenticated unit's tag.
	unitTag string
}

// NewState creates a new client-side Uniter facade.
func NewState(caller base.Caller, authTag string) *State {
	return &State{
		EnvironWatcher: common.NewEnvironWatcher(uniter, caller),
		caller:         caller,
		unitTag:        authTag}
}

// life requests the lifecycle of the given entity from the server.
func (st *State) life(tag string) (params.Life, error) {
	return common.Life(st.caller, uniter, tag)
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
	err := st.caller.Call(uniter, "", "Relation", args, &result)
	if err != nil {
		return nothing, err
	}
	if len(result.Results) != 1 {
		return nothing, fmt.Errorf("expected one result, got %d", len(result.Results))
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
	err := st.caller.Call(uniter, "", "ProviderType", nil, &result)
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

// Relation returns the existing relation with the given tag.
func (st *State) RelationById(id int) (*Relation, error) {
	var results params.RelationResults
	args := params.RelationIds{
		RelationIds: []int{id},
	}
	err := st.caller.Call(uniter, "", "RelationById", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected one result, got %d", len(results.Results))
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
	return &Environment{st}, nil
}

// APIAddresses returns the list of addresses used to connect to the API.
func (st *State) APIAddresses() ([]string, error) {
	var result params.StringsResult
	err := st.caller.Call(uniter, "", "APIAddresses", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, err
	}
	return result.Result, nil
}
