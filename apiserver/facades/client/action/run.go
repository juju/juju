// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"time"

	"github.com/juju/collections/transform"

	"github.com/juju/names/v6"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	coreoperation "github.com/juju/juju/core/operation"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// Run the commands specified on the machines identified through the
// list of machines, units and services.
func (a *ActionAPI) Run(ctx context.Context, run params.RunParams) (results params.EnqueuedActions, err error) {
	if err := a.checkCanAdmin(ctx); err != nil {
		return results, err
	}

	if err := a.check.ChangeAllowed(ctx); err != nil {
		return results, errors.Capture(err)
	}

	target := makeOperationTarget(run.Applications, run.Machines, run.Units)
	args, err := newOperationTaskParams(run.Commands, run.Timeout, run.Parallel, run.ExecutionGroup)
	if err != nil {
		return results, errors.Capture(err)
	}

	result, err := a.operationService.Run(ctx, []operation.RunArgs{{
		Target:   target,
		TaskArgs: args,
	}})
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

	args, err := newOperationTaskParams(run.Commands, run.Timeout, run.Parallel, run.ExecutionGroup)
	if err != nil {
		return results, errors.Capture(err)
	}

	result, err := a.operationService.RunOnAllMachines(ctx, args)
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

func newOperationTaskParams(
	quotedCommands string,
	timeout time.Duration,
	parallel *bool,
	executionGroup *string,
) (operation.TaskArgs, error) {
	if coreoperation.HasJujuExecAction(quotedCommands) {
		return operation.TaskArgs{}, errors.Errorf("cannot use %q as an action command",
			quotedCommands).Add(coreerrors.NotSupported)
	}

	actionParams := map[string]interface{}{}
	actionParams["command"] = quotedCommands
	actionParams["timeout"] = timeout.Nanoseconds()

	return operation.TaskArgs{
		ActionName:     coreoperation.JujuExecActionName,
		Parameters:     actionParams,
		IsParallel:     zeroNilPtr(parallel),
		ExecutionGroup: zeroNilPtr(executionGroup),
	}, nil
}
