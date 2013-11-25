// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"

	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
)

// State provides access to the deployer worker's idea of the state.
type State struct {
	caller common.Caller
}

// NewState creates a new State instance that makes API calls
// through the given caller.
func NewState(caller common.Caller) *State {
	return &State{caller}
}

// unitLife returns the lifecycle state of the given unit.
func (st *State) unitLife(tag string) (params.Life, error) {
	var result params.LifeResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := st.caller.Call("Deployer", "", "Life", args, &result)
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

// Unit returns the unit with the given tag.
func (st *State) Unit(tag string) (*Unit, error) {
	life, err := st.unitLife(tag)
	if err != nil {
		return nil, err
	}
	return &Unit{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}

// Machine returns the machine with the given tag.
func (st *State) Machine(tag string) (*Machine, error) {
	return &Machine{
		tag: tag,
		st:  st,
	}, nil
}

// StateAddresses returns the list of addresses used to connect to the state.
func (st *State) StateAddresses() ([]string, error) {
	var result params.StringsResult
	err := st.caller.Call("Deployer", "", "StateAddresses", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Result, nil
}

// APIAddresses returns the list of addresses used to connect to the API.
func (st *State) APIAddresses() ([]string, error) {
	var result params.StringsResult
	err := st.caller.Call("Deployer", "", "APIAddresses", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Result, nil
}

// CACert returns the certificate used to validate the state connection.
func (st *State) CACert() ([]byte, error) {
	var result params.BytesResult
	err := st.caller.Call("Deployer", "", "CACert", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Result, nil
}

// ConnectionInfo returns all the address information that the deployer task
// needs in one call.
func (st *State) ConnectionInfo() (result params.DeployerConnectionValues, err error) {
	err = st.caller.Call("Deployer", "", "ConnectionInfo", nil, &result)
	return result, err
}
