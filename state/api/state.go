// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"launchpad.net/juju-core/state/api/machiner"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/upgrader"
)

// Login authenticates as the entity with the given name and password.
// Subsequent requests on the state will act as that entity.
// This method is usually called automatically by Open.
func (st *State) Login(tag, password string) error {
	return st.Call("Admin", "", "Login", &params.Creds{
		AuthTag:  tag,
		Password: password,
	}, nil)
}

// Client returns an object that can be used
// to access client-specific functionality.
func (st *State) Client() *Client {
	return &Client{st}
}

// Machiner returns an object that can be used to access the Machiner
// API facade.
func (st *State) Machiner() (*machiner.Machiner, error) {
	// Just verify we're allowed to access it.
	args := params.Machines{
		Ids: []string{},
	}
	var result params.MachinesLifeResults
	err := st.Call("Machiner", "", "Life", args, &result)
	if err != nil {
		return nil, err
	}
	return machiner.New(st), nil
}

// Upgrader returns access to the Upgrader API
func (st *State) Upgrader() (*upgrader.Upgrader, error) {
	return &upgrader.Upgrader{}, nil
}
