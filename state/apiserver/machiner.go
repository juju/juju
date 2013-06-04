// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// srvMachiner represents the Machiner API facade used by the machiner worker.
type srvMachiner struct {
	st   *state.State
	auth Authorizer
}

// SetStatus sets the status of each given machine.
func (m *srvMachiner) SetStatus(args params.MachinesSetStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Errors: make([]*params.Error, len(args.Machines)),
	}
	if len(args.Machines) == 0 {
		return result, nil
	}
	for i, arg := range args.Machines {
		machine, err := m.st.Machine(arg.Id)
		if err == nil {
			// Allow only for the owner agent or the environment manager
			if !m.auth.AuthOwner(machine) {
				err = errPerm
			} else {
				err = machine.SetStatus(arg.Status, arg.Info)
			}
		}
		result.Errors[i] = serverErrorToParams(err)
	}
	return result, nil
}

// Watch starts an EntityWatcher for each given machine.
//func (m *srvMachiner) Watch(args params.Machines) (params.MachinerWatchResults, error) {
// TODO (dimitern) implement this once the watchers can handle bulk ops
//}

// Life returns the lifecycle state of each given machine.
func (m *srvMachiner) Life(args params.Machines) (params.MachinesLifeResults, error) {
	result := params.MachinesLifeResults{
		Machines: make([]params.MachineLifeResult, len(args.Ids)),
	}
	if len(args.Ids) == 0 {
		return result, nil
	}
	for i, id := range args.Ids {
		machine, err := m.st.Machine(id)
		if err == nil {
			// Allow only for the owner agent or the environment manager
			if !m.auth.AuthOwner(machine) {
				err = errPerm
			} else {
				result.Machines[i].Life = params.Life(machine.Life().String())
			}
		}
		result.Machines[i].Error = serverErrorToParams(err)
	}
	return result, nil
}

// EnsureDead changes the lifecycle of each given machine to Dead if
// it's Alive or Dying. It does nothing otherwise.
func (m *srvMachiner) EnsureDead(args params.Machines) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Errors: make([]*params.Error, len(args.Ids)),
	}
	if len(args.Ids) == 0 {
		return result, nil
	}
	for i, id := range args.Ids {
		machine, err := m.st.Machine(id)
		if err == nil {
			// Allow only for the owner agent or the environment manager
			if !m.auth.AuthOwner(machine) {
				err = errPerm
			} else {
				err = machine.EnsureDead()
			}
		}
		result.Errors[i] = serverErrorToParams(err)
	}
	return result, nil
}
