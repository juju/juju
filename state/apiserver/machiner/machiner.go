// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	apicommon "launchpad.net/juju-core/state/apiserver/common"
)

// Machiner represents the Machiner API facade used by the machiner worker.
type Machiner struct {
	st   *state.State
	auth apicommon.Authorizer
}

// New creates a new instance of the Machiner facade.
func New(st *state.State, authorizer apicommon.Authorizer) *Machiner {
	return &Machiner{st, authorizer}
}

// SetStatus sets the status of each given machine.
func (m *Machiner) SetStatus(args params.MachinesSetStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Errors: make([]*params.Error, len(args.Machines)),
	}
	if len(args.Machines) == 0 {
		return result, nil
	}
	for i, arg := range args.Machines {
		machine, err := m.st.Machine(arg.Id)
		if err == nil {
			// Allow only for the owner agent.
			if !m.auth.AuthOwner(machine) {
				err = apicommon.ErrPerm
			} else {
				err = machine.SetStatus(arg.Status, arg.Info)
			}
		}
		result.Errors[i] = apicommon.ServerErrorToParams(err)
	}
	return result, nil
}

// Watch starts an EntityWatcher for each given machine.
//func (m *Machiner) Watch(args params.Machines) (params.MachinerWatchResults, error) {
// TODO (dimitern) implement this once the watchers can handle bulk ops
//}

// Life returns the lifecycle state of each given machine.
func (m *Machiner) Life(args params.Machines) (params.MachinesLifeResults, error) {
	result := params.MachinesLifeResults{
		Machines: make([]params.MachineLifeResult, len(args.Ids)),
	}
	if len(args.Ids) == 0 {
		return result, nil
	}
	for i, id := range args.Ids {
		machine, err := m.st.Machine(id)
		if err == nil {
			// Allow only for the owner agent.
			if !m.auth.AuthOwner(machine) {
				err = apicommon.ErrPerm
			} else {
				result.Machines[i].Life = params.Life(machine.Life().String())
			}
		}
		result.Machines[i].Error = apicommon.ServerErrorToParams(err)
	}
	return result, nil
}

// EnsureDead changes the lifecycle of each given machine to Dead if
// it's Alive or Dying. It does nothing otherwise.
func (m *Machiner) EnsureDead(args params.Machines) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Errors: make([]*params.Error, len(args.Ids)),
	}
	if len(args.Ids) == 0 {
		return result, nil
	}
	for i, id := range args.Ids {
		machine, err := m.st.Machine(id)
		if err == nil {
			// Allow only for the owner agent.
			if !m.auth.AuthOwner(machine) {
				err = apicommon.ErrPerm
			} else {
				err = machine.EnsureDead()
			}
		}
		result.Errors[i] = apicommon.ServerErrorToParams(err)
	}
	return result, nil
}
