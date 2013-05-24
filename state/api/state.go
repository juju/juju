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

// Machine returns a reference to the machine with the given id.
func (st *State) Machine(id string) (*Machine, error) {
	m := &Machine{
		st: st,
		id: id,
	}
	if err := m.Refresh(); err != nil {
		return nil, err
	}
	return m, nil
}

// Unit returns a unit by name.
func (st *State) Unit(name string) (*Unit, error) {
	u := &Unit{
		st:   st,
		name: name,
	}
	if err := u.Refresh(); err != nil {
		return nil, err
	}
	return u, nil
}

// AllMachines returns all machines in the environment
// ordered by id.
func (st *State) AllMachines() ([]*Machine, error) {
	var results params.AllMachinesResults
	err := st.call("State", "", "AllMachines", nil, &results)
	if err != nil {
		return nil, err
	}
	machines := make([]*Machine, len(results.Machines))
	for i, m := range results.Machines {
		machines[i] = &Machine{
			st:  st,
			id:  m.Id,
			doc: *m,
		}
	}
	return machines, nil
}

// WatchMachines returns a LifecycleWatcher that notifies of changes to
// the lifecycles of the machines in the environment.
func (st *State) WatchMachines() *LifecycleWatcher {
	return newLifecycleWatcher(st, "WatchMachines")
}

// WatchEnvironConfig returns a watcher for observing changes
// to the environment configuration.
func (st *State) WatchEnvironConfig() *EnvironConfigWatcher {
	return newEnvironConfigWatcher(st)
}
