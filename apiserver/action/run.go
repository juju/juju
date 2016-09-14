// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/state"
)

// getAllUnitNames returns a sequence of valid Unit objects from state. If any
// of the service names or unit names are not found, an error is returned.
func getAllUnitNames(st *state.State, units, services []string) (result []names.Tag, err error) {
	unitsSet := set.NewStrings(units...)
	for _, name := range services {
		service, err := st.Application(name)
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
	for _, unitName := range unitsSet.SortedValues() {
		if !names.IsValidUnit(unitName) {
			return nil, errors.Errorf("invalid unit name %q", unitName)
		}
		result = append(result, names.NewUnitTag(unitName))
	}
	return result, nil
}

// Run the commands specified on the machines identified through the
// list of machines, units and services.
func (a *ActionAPI) Run(run params.RunParams) (results params.ActionResults, err error) {
	if err := a.checkCanAdmin(); err != nil {
		return results, err
	}
	if err := a.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}

	units, err := getAllUnitNames(a.state, run.Units, run.Applications)
	if err != nil {
		return results, errors.Trace(err)
	}

	machines := make([]names.Tag, len(run.Machines))
	for i, machineId := range run.Machines {
		if !names.IsValidMachine(machineId) {
			return results, errors.Errorf("invalid machine id %q", machineId)
		}
		machines[i] = names.NewMachineTag(machineId)
	}

	actionParams := a.createActionsParams(append(units, machines...), run.Commands, run.Timeout)

	return queueActions(a, actionParams)
}

// RunOnAllMachines attempts to run the specified command on all the machines.
func (a *ActionAPI) RunOnAllMachines(run params.RunParams) (results params.ActionResults, err error) {
	if err := a.checkCanAdmin(); err != nil {
		return results, err
	}

	if err := a.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}

	machines, err := a.state.AllMachines()
	if err != nil {
		return results, err
	}
	machineTags := make([]names.Tag, len(machines))
	for i, machine := range machines {
		machineTags[i] = machine.Tag()
	}

	actionParams := a.createActionsParams(machineTags, run.Commands, run.Timeout)

	return queueActions(a, actionParams)
}

func (a *ActionAPI) createActionsParams(actionReceiverTags []names.Tag, quotedCommands string, timeout time.Duration) params.Actions {

	apiActionParams := params.Actions{Actions: []params.Action{}}

	actionParams := map[string]interface{}{}
	actionParams["command"] = quotedCommands
	actionParams["timeout"] = timeout.Nanoseconds()

	for _, tag := range actionReceiverTags {
		apiActionParams.Actions = append(apiActionParams.Actions, params.Action{
			Receiver:   tag.String(),
			Name:       actions.JujuRunActionName,
			Parameters: actionParams,
		})
	}

	return apiActionParams
}

var queueActions = func(a *ActionAPI, args params.Actions) (results params.ActionResults, err error) {
	return a.Enqueue(args)
}
