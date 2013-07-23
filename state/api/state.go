// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"launchpad.net/juju-core/state/api/deployer"
	"launchpad.net/juju-core/state/api/machineagent"
	"launchpad.net/juju-core/state/api/machiner"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/upgrader"
)

// Login authenticates as the entity with the given name and password.
// Subsequent requests on the state will act as that entity.  This
// method is usually called automatically by Open. The machine nonce
// should be empty unless logging in as a machine agent.
func (st *State) Login(tag, password, nonce string) error {
	return st.Call("Admin", "", "Login", &params.Creds{
		AuthTag:  tag,
		Password: password,
		Nonce:    nonce,
	}, nil)
}

// Client returns an object that can be used
// to access client-specific functionality.
func (st *State) Client() *Client {
	return &Client{st}
}

// Machiner returns a version of the state that provides functionality
// required by the machiner worker.
func (st *State) Machiner() *machiner.State {
	return machiner.NewState(st)
}

// MachineAgent returns a version of the state that provides
// functionality required by the machine agent code.
func (st *State) MachineAgent() *machineagent.State {
	return machineagent.NewState(st)
}

// Upgrader returns access to the Upgrader API
func (st *State) Upgrader() (*upgrader.State, error) {
	return upgrader.NewState(st), nil
}

// Deployer returns access to the Deployer API
func (st *State) Deployer() (*deployer.State, error) {
	return deployer.NewState(st), nil
}
