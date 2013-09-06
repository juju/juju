// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
)

// State provides access to the Uniter API facade.
type State struct {
	caller common.Caller
	// unitTag contains the authenticated unit's tag.
	unitTag string
}

// NewState creates a new client-side Uniter facade.
func NewState(caller common.Caller, authTag string) *State {
	return &State{caller, authTag}
}

// life requests the lifecycle of the given entity from the server.
func (st *State) life(tag string) (params.Life, error) {
	var result params.LifeResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := st.caller.Call("Uniter", "", "Life", args, &result)
	if err != nil {
		return "", err
	}
	if len(result.Results) != 1 {
		return "", fmt.Errorf("expected one result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return "", err
	}
	return result.Results[0].Life, nil
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
	err := st.caller.Call("Uniter", "", "Relation", args, &result)
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
	// TODO: Return a new uniter.Service proxy object for tag.
	panic("not implemented")
}

// ProviderType returns a provider type used by the current juju
// environment.
//
// TODO(dimitern): We might be able to drop this, once we have machine
// addresses implemented fully. See also LP bug 1221798.
func (st *State) ProviderType() string {
	// TODO: Call Uniter.ProviderType()
	panic("not implemented")
}

// Charm returns the charm with the given URL.
func (st *State) Charm(curl *charm.URL) (*Charm, error) {
	// TODO: Return a new uniter.Service proxy object for tag.
	panic("not implemented")
}

// Relation returns the existing relation with the given tag.
func (st *State) Relation(tag string) (*Relation, error) {
	// Get the life, then get the id and other info.
	life, err := st.life(tag)
	if err != nil {
		return nil, err
	}
	result, err := st.relation(tag, st.unitTag)
	if err != nil {
		return nil, err
	}
	return &Relation{
		id:   result.Id,
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}

// Environment returns the environment entity.
func (st *State) Environment() (*Environment, error) {
	return &Environment{st}, nil
}
