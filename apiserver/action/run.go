// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/state"
)

// getAllUnitNames returns a sequence of valid Unit objects from state. If any
// of the service names or unit names are not found, an error is returned.
func getAllUnitNames(st *state.State, units, services []string) (result []*state.Unit, err error) {
	unitsSet := set.NewStrings(units...)
	for _, name := range services {
		service, err := st.Service(name)
		if err != nil {
			return nil, err
		}
		units, err := service.AllUnits()
		if err != nil {
			return nil, err
		}
		for _, unit := range units {
			unitsSet.Add(unit.Name())
		}
	}
	for _, unitName := range unitsSet.Values() {
		unit, err := st.Unit(unitName)
		if err != nil {
			return nil, err
		}
		result = append(result, unit)
	}
	return result, nil
}

// Run the commands specified on the machines identified through the
// list of machines, units and services.
func (a *ActionAPI) Run(run params.RunParams) (results params.ActionResults, err error) {
	if err := a.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}

	units, err := getAllUnitNames(a.state, run.Units, run.Services)
	if err != nil {
		return results, errors.Trace(err)
	}

	machines := make([]*state.Machine, len(run.Machines))
	for i, machineId := range run.Machines {
		machines[i], err = a.state.Machine(machineId)
		if err != nil {
			return results, err
		}
	}

	actionParams := a.createActionsParams(units, machines, run.Commands, run.Timeout)

	return queueActions(a, actionParams)
}

// RunOnAllMachines attempts to run the specified command on all the machines.
func (a *ActionAPI) RunOnAllMachines(run params.RunParams) (results params.ActionResults, err error) {
	if err := a.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}

	machines, err := a.state.AllMachines()
	if err != nil {
		return results, err
	}

	actionParams := a.createActionsParams([]*state.Unit{}, machines, run.Commands, run.Timeout)

	return queueActions(a, actionParams)
}

func (a *ActionAPI) createActionsParams(units []*state.Unit, machines []*state.Machine, quotedCommands string, timeout time.Duration) params.Actions {

	apiActionParams := params.Actions{Actions: []params.Action{}}

	actionParams := map[string]interface{}{}
	actionParams["command"] = quotedCommands
	actionParams["timeout"] = timeout.Nanoseconds()

	for _, unit := range units {
		apiActionParams.Actions = append(apiActionParams.Actions, params.Action{
			Receiver:   unit.Tag().String(),
			Name:       actions.JujuRunActionName,
			Parameters: actionParams,
		})
	}

	for _, machine := range machines {
		apiActionParams.Actions = append(apiActionParams.Actions, params.Action{
			Receiver:   machine.Tag().String(),
			Name:       actions.JujuRunActionName,
			Parameters: actionParams,
		})
	}

	return apiActionParams
}

var queueActions = func(a *ActionAPI, args params.Actions) (results params.ActionResults, err error) {
	return a.Enqueue(args)
}
