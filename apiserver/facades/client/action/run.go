// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// getAllUnitNames returns a sequence of valid Unit objects from state. If any
// of the application names or unit names are not found, an error is returned.
func getAllUnitNames(st State, units, applications []string) (result []names.Tag, err error) {
	var leaders map[string]string
	getLeader := func(appName string) (string, error) {
		if leaders == nil {
			var err error
			leaders, err = st.ApplicationLeaders()
			if err != nil {
				return "", err
			}
		}
		if leader, ok := leaders[appName]; ok {
			return leader, nil
		}
		return "", errors.Errorf("could not determine leader for %q", appName)
	}

	// Replace units matching $app/leader with the appropriate unit for
	// the leader.
	unitsSet := set.NewStrings()
	for _, unit := range units {
		if !strings.HasSuffix(unit, "leader") {
			unitsSet.Add(unit)
			continue
		}

		app := strings.Split(unit, "/")[0]
		leaderUnit, err := getLeader(app)
		if err != nil {
			return nil, apiservererrors.ServerError(err)
		}

		unitsSet.Add(leaderUnit)
	}

	for _, name := range applications {
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
func (a *ActionAPI) Run(run params.RunParams) (results params.EnqueuedActions, err error) {
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

	actionParams, err := a.createRunActionsParams(append(units, machines...), run.Commands, run.Timeout, run.WorkloadContext, run.Parallel, run.ExecutionGroup)
	if err != nil {
		return results, errors.Trace(err)
	}
	return a.EnqueueOperation(actionParams)
}

// RunOnAllMachines attempts to run the specified command on all the machines.
func (a *ActionAPI) RunOnAllMachines(run params.RunParams) (results params.EnqueuedActions, err error) {
	if err := a.checkCanAdmin(); err != nil {
		return results, err
	}

	if err := a.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}

	m, err := a.state.Model()
	if err != nil {
		return results, errors.Trace(err)
	}
	if m.Type() != state.ModelTypeIAAS {
		return results, errors.Errorf("cannot run on all machines with a %s model", m.Type())
	}

	machines, err := a.state.AllMachines()
	if err != nil {
		return results, err
	}
	machineTags := make([]names.Tag, len(machines))
	for i, machine := range machines {
		machineTags[i] = machine.Tag()
	}

	actionParams, err := a.createRunActionsParams(machineTags, run.Commands, run.Timeout, false, run.Parallel, run.ExecutionGroup)
	if err != nil {
		return results, errors.Trace(err)
	}
	return a.EnqueueOperation(actionParams)
}

func (a *ActionAPI) createRunActionsParams(
	actionReceiverTags []names.Tag,
	quotedCommands string,
	timeout time.Duration,
	workloadContext bool,
	parallel *bool,
	executionGroup *string,
) (params.Actions, error) {
	apiActionParams := params.Actions{Actions: []params.Action{}}

	if actions.HasJujuExecAction(quotedCommands) {
		return apiActionParams, errors.NewNotSupported(nil, fmt.Sprintf("cannot use %q as an action command", quotedCommands))
	}

	actionParams := map[string]interface{}{}
	actionParams["command"] = quotedCommands
	actionParams["timeout"] = timeout.Nanoseconds()
	actionParams["workload-context"] = workloadContext

	for _, tag := range actionReceiverTags {
		apiActionParams.Actions = append(apiActionParams.Actions, params.Action{
			Receiver:       tag.String(),
			Name:           actions.JujuExecActionName,
			Parameters:     actionParams,
			Parallel:       parallel,
			ExecutionGroup: executionGroup,
		})
	}

	return apiActionParams, nil
}
