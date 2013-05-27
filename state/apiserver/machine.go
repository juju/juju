// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	statewatcher "launchpad.net/juju-core/state/watcher"
)

// srvMachine serves API methods on a machine.
type srvMachine struct {
	root *srvRoot
	m    *state.Machine
}

// Get retrieves all the details of a machine.
func (m *srvMachine) Get() params.Machine {
	machine := stateMachineToParams(m.m)
	return *machine
}

func (m *srvMachine) Watch() (params.EntityWatcherId, error) {
	watcher := m.m.Watch()
	// To save an extra round-trip to call Next after Watch, we check
	// for initial changes.
	if _, ok := <-watcher.Changes(); !ok {
		return params.EntityWatcherId{}, statewatcher.MustErr(watcher)
	}
	return params.EntityWatcherId{
		EntityWatcherId: m.root.resources.register(watcher).id,
	}, nil
}

// SetAgentAlive signals that the agent for machine m is alive.
func (m *srvMachine) SetAgentAlive() (params.PingerId, error) {
	if !m.root.authOwner(m.m) {
		return params.PingerId{}, errPerm
	}
	pinger, err := m.m.SetAgentAlive()
	if err != nil {
		return params.PingerId{}, err
	}
	return params.PingerId{
		PingerId: m.root.resources.register(pinger).id,
	}, nil
}

// EnsureDead sets the machine lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise. See machine.EnsureDead().
func (m *srvMachine) EnsureDead() error {
	if !m.root.authOwner(m.m) && !m.root.authEnvironManager() {
		return errPerm
	}
	return m.m.EnsureDead()
}

// Remove removes the machine from state. It will fail if the machine
// is not Dead.
func (m *srvMachine) Remove() error {
	if !m.root.authEnvironManager() {
		return errPerm
	}
	return m.m.Remove()
}

// Constraints returns the exact constraints that should apply when
// provisioning an instance for the machine.
func (m *srvMachine) Constraints() (params.ConstraintsResults, error) {
	if !m.root.authEnvironManager() {
		return params.ConstraintsResults{}, errPerm
	}
	constraints, err := m.m.Constraints()
	if err != nil {
		return params.ConstraintsResults{}, err
	}
	return params.ConstraintsResults{
		Constraints: constraints,
	}, nil
}

// SetProvisioned sets the provider specific machine id and nonce for
// this machine. Once set, the instance id cannot be changed.
func (m *srvMachine) SetProvisioned(args params.SetProvisioned) error {
	if !m.root.authEnvironManager() {
		return errPerm
	}
	return m.m.SetProvisioned(
		state.InstanceId(args.InstanceId),
		args.Nonce,
	)
}

// Status returns the status of the machine.
func (m *srvMachine) Status() (params.StatusResults, error) {
	if !m.root.authEnvironManager() {
		return params.StatusResults{}, errPerm
	}
	status, info, err := m.m.Status()
	if err != nil {
		return params.StatusResults{}, err
	}
	return params.StatusResults{
		Status: status,
		Info:   info,
	}, nil
}

// SetStatus sets the status of the machine.
func (m *srvMachine) SetStatus(status params.SetStatus) error {
	if !m.root.authOwner(m.m) && !m.root.authEnvironManager() {
		return errPerm
	}
	return m.m.SetStatus(status.Status, status.Info)
}

// SetPassword sets the machine's password.
func (m *srvMachine) SetPassword(p params.Password) error {
	if !m.root.authOwner(m.m) && !m.root.authEnvironManager() {
		return errPerm
	}
	if err := setPassword(m.m, p.Password); err != nil {
		return err
	}
	// Grant access to the mongo state if the machine requires it.
	if isMachineWithJob(m.m, state.JobManageEnviron) ||
		isMachineWithJob(m.m, state.JobServeAPI) {
		return m.m.SetMongoPassword(p.Password)
	}
	return nil
}
