// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"reflect"
	"strings"

	"github.com/juju/collections/transform"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	coreoperation "github.com/juju/juju/core/operation"
	corestatus "github.com/juju/juju/core/status"
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
	receivers := make([]operation.ActionReceiver, 0, len(arg.Actions))

	// Validate that all actions have the same parameters, modulo the receiver.
	// Note: this is a change of behavior from Juju 3, which allows different
	// actions to be run as one operation. However, the only known user of this
	// API is the Juju CLI, which actually only runs one action at a time, on
	// several receivers.
	taskParams, err := a.validate(arg)
	if err != nil {
		return params.EnqueuedActions{}, errors.Capture(err)
	}

	// We are going to check that all the provided action targets are units
	// from the same application.
	var applicationName string
	for i, action := range arg.Actions {

		if strings.HasSuffix(action.Receiver, leader) {
			receiver := strings.TrimSuffix(action.Receiver, leader)

			// Now check if the leader application is the same as the previous
			// ones.
			if applicationName == "" {
				applicationName = receiver
			} else if applicationName != receiver {
				return params.EnqueuedActions{}, errors.New("actions must be run on units from the same application")
			}

			actionResultByUnitName[action.Receiver] = &actionResults[i]
			receivers = append(receivers, operation.ActionReceiver{LeaderUnit: receiver})

			continue
		}

		unitTag, err := names.ParseUnitTag(action.Receiver)
		if err != nil {
			actionResults[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// Now check if the application is the same as the previous ones.
		if applicationName == "" {
			applicationName = unit.Name(unitTag.Id()).Application()
		} else if applicationName != unit.Name(unitTag.Id()).Application() {
			return params.EnqueuedActions{}, errors.New("actions must be run on units from the same application")
		}

		actionResultByUnitName[unitTag.Id()] = &actionResults[i]
		receivers = append(receivers, operation.ActionReceiver{Unit: unit.Name(unitTag.Id())})
	}

	// If no valid receivers (all are invalid), do not call service; return per-action errors.
	if len(receivers) == 0 {
		return params.EnqueuedActions{Actions: actionResults}, nil
	}

	result, err := a.operationService.AddActionOperation(ctx, receivers, taskParams)
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

// validate validates that all actions have the same parameters, modulo the receiver.
func (*ActionAPI) validate(arg params.Actions) (operation.TaskArgs, error) {
	var result *operation.TaskArgs

	for _, action := range arg.Actions {
		incoming := operation.TaskArgs{
			ActionName:     action.Name,
			Parameters:     action.Parameters,
			IsParallel:     zeroNilPtr(action.Parallel),
			ExecutionGroup: zeroNilPtr(action.ExecutionGroup),
		}

		if result == nil {
			result = &incoming
			continue
		}

		var errs []error
		if result.ActionName != incoming.ActionName {
			errs = append(errs, errors.Errorf("action name mismatch: %q != %q", result.ActionName,
				incoming.ActionName))
		}
		if result.IsParallel != incoming.IsParallel {
			errs = append(errs, errors.Errorf("parallel mismatch: %v != %v", result.IsParallel,
				incoming.IsParallel))
		}
		if result.ExecutionGroup != incoming.ExecutionGroup {
			errs = append(errs, errors.Errorf("execution group mismatch: %v != %v", result.ExecutionGroup,
				incoming.ExecutionGroup))
		}
		if !reflect.DeepEqual(result.Parameters, incoming.Parameters) {
			errs = append(errs, errors.Errorf("parameters mismatch: %v != %v", result.Parameters,
				incoming.Parameters))
		}
		return *result, errors.Join(errs...)
	}

	if result == nil {
		return operation.TaskArgs{}, errors.New("no actions specified")
	}

	return *result, nil
}

// ListOperations fetches the called operations for specified apps/units.
func (a *ActionAPI) ListOperations(ctx context.Context, arg params.OperationQueryArgs) (params.OperationResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.OperationResults{}, errors.Capture(err)
	}

	var status []corestatus.Status
	if arg.Status != nil {
		status = transform.Slice(arg.Status, func(f string) corestatus.Status {
			return corestatus.Status(f)
		})
	}
	args := operation.QueryArgs{
		Receivers:   makeOperationReceivers(arg.Applications, arg.Machines, arg.Units),
		ActionNames: arg.ActionNames,
		Status:      status,
		Limit:       arg.Limit,
		Offset:      arg.Offset,
	}
	result, err := a.operationService.GetOperations(ctx, args)
	if err != nil {
		return params.OperationResults{}, errors.Capture(err)
	}

	return toOperationResults(result), errors.Capture(err)
}

// Operations fetches the specified operation ids.
func (a *ActionAPI) Operations(ctx context.Context, arg params.Entities) (params.OperationResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.OperationResults{}, errors.Capture(err)
	}

	results := params.OperationResults{Results: make([]params.OperationResult, len(arg.Entities))}

	for i, entity := range arg.Entities {
		operationTag, err := names.ParseOperationTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result, err := a.operationService.GetOperationByID(ctx, operationTag.Id())
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i] = toOperationResult(result)
	}
	return results, nil
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

	result, err := a.operationService.AddExecOperation(ctx, target, args)
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

	result, err := a.operationService.AddExecOperationOnAllMachines(ctx, args)
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
