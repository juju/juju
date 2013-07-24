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
	return &State{caller: caller}
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

// serverInfo returns the state and API addresses, as well as the CA
// certificate.
func (st *State) serverInfo() (params.ServerInfoResult, error) {
	var result params.ServerInfoResult
	err := st.caller.Call("Deployer", "", "ServerInfo", nil, &result)
	return result, err
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

// Addresses returns the list of addresses used to connect to the state.
func (st *State) Addresses() ([]string, error) {
	result, err := st.serverInfo()
	if err != nil {
		return nil, err
	}
	return result.Addresses, nil
}

// APIAddresses returns the list of addresses used to connect to the API.
func (st *State) APIAddresses() ([]string, error) {
	result, err := st.serverInfo()
	if err != nil {
		return nil, err
	}
	return result.APIAddresses, nil
}

// CACert returns the certificate used to validate the state connection.
func (st *State) CACert() ([]byte, error) {
	result, err := st.serverInfo()
	if err != nil {
		return nil, err
	}
	return result.CACert, nil
}
