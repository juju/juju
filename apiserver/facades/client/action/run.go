// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"strings"

	"github.com/juju/collections/transform"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	coreoperation "github.com/juju/juju/core/operation"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// EnqueueOperation takes a list of Actions and queues them up to be executed as
// an operation, each action running as a task on the designated ActionReceiver.
// We return the ID of the overall operation and each individual task.
func (a *ActionAPI) EnqueueOperation(ctx context.Context, arg params.Actions) (params.EnqueuedActions, error) {
	if err := a.checkCanWrite(ctx); err != nil {
		return params.EnqueuedActions{}, errors.Capture(err)
	}

	if len(arg.Actions) == 0 {
		return params.EnqueuedActions{}, apiservererrors.ServerError(errors.New("no actions specified"))
	}

	const leader = "/leader"
	actionResults := make([]params.ActionResult, len(arg.Actions))
	actionResultByUnitName := make(map[string]*params.ActionResult)
	runParams := make([]operation.ActionArgs, 0, len(arg.Actions))
	for i, action := range arg.Actions {
		taskParams := operation.TaskArgs{
			ActionName:     action.Name,
			Parameters:     action.Parameters,
			IsParallel:     zeroNilPtr(action.Parallel),
			ExecutionGroup: zeroNilPtr(action.ExecutionGroup),
		}

		if strings.HasSuffix(action.Receiver, leader) {
			receiver := strings.TrimSuffix(action.Receiver, leader)
			actionResultByUnitName[action.Receiver] = &actionResults[i]
			runParams = append(runParams, operation.ActionArgs{
				ActionReceiver: operation.ActionReceiver{LeaderUnit: receiver},
				TaskArgs:       taskParams,
			})
			continue
		}

		unitTag, err := names.ParseUnitTag(action.Receiver)
		if err != nil {
			actionResults[i].Error = apiservererrors.ServerError(err)
			continue
		}
		actionResultByUnitName[unitTag.Id()] = &actionResults[i]
		runParams = append(runParams, operation.ActionArgs{
			ActionReceiver: operation.ActionReceiver{
				Unit: unit.Name(unitTag.Id()),
			},
			TaskArgs: taskParams,
		})

	}

	// If no valid run params (all receivers invalid), do not call service; return per-action errors.
	if len(runParams) == 0 {
		return params.EnqueuedActions{Actions: actionResults}, nil
	}

	result, err := a.operationService.StartActionOperation(ctx, runParams)
	if err != nil {
		return params.EnqueuedActions{}, errors.Capture(err)
	}

	for _, unitResult := range result.Units {
		targetName := unitResult.ReceiverName.String()
		if unitResult.IsLeader {
			targetName = unitResult.ReceiverName.Application() + leader
		}
		ar := actionResultByUnitName[targetName]
		if ar == nil {
			return params.EnqueuedActions{}, errors.Errorf("unexpected result for %q", targetName)
		}
		*ar = toActionResult(names.NewUnitTag(unitResult.ReceiverName.String()), unitResult.TaskInfo)
	}
	// Mark missing results
	for key, ar := range actionResultByUnitName {
		if ar != nil && ar.Action == nil && ar.Error == nil {
			ar.Error = apiservererrors.ServerError(errors.Errorf("missing result for %q", key).Add(coreerrors.NotFound))
		}
	}
	return params.EnqueuedActions{
		OperationTag: names.NewOperationTag(result.OperationID).String(),
		Actions:      actionResults,
	}, nil
}

// Run the commands specified on the machines identified through the
// list of machines, units and services.
func (a *ActionAPI) Run(ctx context.Context, run params.RunParams) (results params.EnqueuedActions, err error) {
	if err := a.checkCanAdmin(ctx); err != nil {
		return results, err
	}

	if err := a.check.ChangeAllowed(ctx); err != nil {
		return results, errors.Capture(err)
	}

	target := makeOperationReceivers(run.Applications, run.Machines, run.Units)
	args, err := newOperationTaskParams(run)
	if err != nil {
		return results, errors.Capture(err)
	}

	result, err := a.operationService.StartExecOperation(ctx, target, args)
	if err != nil {
		return results, errors.Capture(err)
	}

	return toEnqueuedActions(result), nil
}

// RunOnAllMachines attempts to run the specified command on all the machines.
func (a *ActionAPI) RunOnAllMachines(ctx context.Context, run params.RunParams) (results params.EnqueuedActions, err error) {
	if err := a.checkCanAdmin(ctx); err != nil {
		return results, err
	}

	if err := a.check.ChangeAllowed(ctx); err != nil {
		return results, errors.Capture(err)
	}

	modelInfo, err := a.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return results, errors.Capture(err)
	}

	if modelInfo.Type != model.IAAS {
		return results, errors.Errorf("cannot run on all machines with a %s model", modelInfo.Type)
	}

	args, err := newOperationTaskParams(run)
	if err != nil {
		return results, errors.Capture(err)
	}

	result, err := a.operationService.StartExecOperationOnAllMachines(ctx, args)
	if err != nil {
		return results, errors.Capture(err)
	}

	return toEnqueuedActions(result), errors.Capture(err)
}

// toEnqueuedActions converts an operation.RunResult to a params.EnqueuedActions.
func toEnqueuedActions(result operation.RunResult) params.EnqueuedActions {

	machineResult := transform.Slice(result.Machines, func(f operation.MachineTaskResult) params.ActionResult {
		result := toActionResult(names.NewMachineTag(f.ReceiverName.String()), f.TaskInfo)
		return result
	})
	unitResults := transform.Slice(result.Units, func(f operation.UnitTaskResult) params.ActionResult {
		result := toActionResult(names.NewUnitTag(f.ReceiverName.String()), f.TaskInfo)
		return result
	})

	return params.EnqueuedActions{
		OperationTag: names.NewOperationTag(result.OperationID).String(),
		Actions:      append(machineResult, unitResults...),
	}
}

// newOperationTaskParams converts a params.RunParams to an operation.ExecArgs.
func newOperationTaskParams(
	run params.RunParams,
) (operation.ExecArgs, error) {
	if coreoperation.HasJujuExecAction(run.Commands) {
		return operation.ExecArgs{}, errors.Errorf("cannot use %q as an action command",
			run.Commands).Add(coreerrors.NotSupported)
	}

	return operation.ExecArgs{
		Command:        run.Commands,
		Timeout:        run.Timeout,
		Parallel:       zeroNilPtr(run.Parallel),
		ExecutionGroup: zeroNilPtr(run.ExecutionGroup),
	}, nil
}
