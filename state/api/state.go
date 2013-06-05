// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import "launchpad.net/juju-core/state/api/params"

// Login authenticates as the entity with the given name and password.
// Subsequent requests on the state will act as that entity.
// This method is usually called automatically by Open.
func (st *State) Login(tag, password string) error {
	return st.call("Admin", "", "Login", &params.Creds{
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
func (st *State) Machiner(version string) (*Machiner, error) {
	// Just verify we're allowed to access it.
	args := params.Machines{
		Ids: []string{},
	}
	var result params.MachinesLifeResults
	err := st.call("Machiner", version, "Life", args, &result)
	if err != nil {
		return nil, err
	}
	return &Machiner{st}, nil
}
