// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/core/model"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/rpc/params"
)

// Run the commands specified on the machines identified through the
// list of machines, units and services.
func (a *ActionAPI) Run(ctx context.Context, run params.RunParams) (results params.EnqueuedActions, err error) {
	if err := a.checkCanAdmin(ctx); err != nil {
		return results, err
	}

	if err := a.check.ChangeAllowed(ctx); err != nil {
		return results, errors.Trace(err)
	}

	units, err := a.getAllUnitNames(ctx, run.Units, run.Applications)
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

	actionParams, err := a.createRunActionsParams(append(units, machines...), run.Commands, run.Timeout, run.Parallel, run.ExecutionGroup)
	if err != nil {
		return results, errors.Trace(err)
	}
	return a.EnqueueOperation(ctx, actionParams)
}

// RunOnAllMachines attempts to run the specified command on all the machines.
func (a *ActionAPI) RunOnAllMachines(ctx context.Context, run params.RunParams) (results params.EnqueuedActions, err error) {
	if err := a.checkCanAdmin(ctx); err != nil {
		return results, err
	}

	if err := a.check.ChangeAllowed(ctx); err != nil {
		return results, errors.Trace(err)
	}

	modelInfo, err := a.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return results, errors.Trace(err)
	}

	if modelInfo.Type != model.IAAS {
		return results, errors.Errorf("cannot run on all machines with a %s model", modelInfo.Type)
	}

	machines, err := a.state.AllMachines()
	if err != nil {
		return results, err
	}
	machineTags := make([]names.Tag, len(machines))
	for i, machine := range machines {
		machineTags[i] = machine.Tag()
	}

	actionParams, err := a.createRunActionsParams(machineTags, run.Commands, run.Timeout, run.Parallel, run.ExecutionGroup)
	if err != nil {
		return results, errors.Trace(err)
	}
	return a.EnqueueOperation(ctx, actionParams)
}

func (a *ActionAPI) createRunActionsParams(
	actionReceiverTags []names.Tag,
	quotedCommands string,
	timeout time.Duration,
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

// getAllUnitNames returns a sequence of valid Unit objects from state. If any
// of the application names or unit names are not found, an error is returned.
func (a *ActionAPI) getAllUnitNames(ctx context.Context, units, applications []string) (result []names.Tag, err error) {
	var leaders map[string]string
	getLeader := func(appName string) (string, error) {
		if leaders == nil {
			var err error
			leaders, err = a.leadership.Leaders()
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

	for _, aName := range applications {
		unitNames, err := a.applicationService.GetUnitNamesForApplication(ctx, aName)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			return nil, errors.NotFoundf("application %q", aName)
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		for _, unitName := range unitNames {
			unitsSet.Add(unitName.String())
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
