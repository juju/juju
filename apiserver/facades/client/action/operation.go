// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"strings"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
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
	runParams := make([]operation.RunArgs, 0, len(arg.Actions))
	for i, action := range arg.Actions {
		taskParams := operation.TaskArgs{
			ActionName:     action.Name,
			Parameters:     action.Parameters,
			IsParallel:     action.Parallel == nil || *action.Parallel, // default is parallel
			ExecutionGroup: zeroNilPtr(action.ExecutionGroup),
		}

		if strings.HasSuffix(action.Receiver, leader) {
			receiver := strings.TrimSuffix(action.Receiver, leader)
			actionResultByUnitName[action.Receiver] = &actionResults[i]
			runParams = append(runParams, operation.RunArgs{
				Target:   operation.Target{LeaderUnit: []string{receiver}},
				TaskArgs: taskParams,
			})
			continue
		}

		unitTag, err := names.ParseUnitTag(action.Receiver)
		if err != nil {
			actionResults[i].Error = apiservererrors.ServerError(err)
			continue
		}
		actionResultByUnitName[unitTag.Id()] = &actionResults[i]
		runParams = append(runParams, operation.RunArgs{
			Target: operation.Target{
				Units: []unit.Name{unit.Name(unitTag.Id())},
			},
			TaskArgs: taskParams,
		})

	}

	// If no valid run params (all receivers invalid), do not call service; return per-action errors.
	if len(runParams) == 0 {
		return params.EnqueuedActions{Actions: actionResults}, nil
	}

	result, err := a.operationService.Run(ctx, runParams)
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
			ar.Error = apiservererrors.ServerError(jujuerrors.NotFoundf("missing result for %q", key))
		}
	}
	return params.EnqueuedActions{
		OperationTag: names.NewOperationTag(result.OperationID).String(),
		Actions:      actionResults,
	}, nil
}

// ListOperations fetches the called actions for specified apps/units.
func (a *ActionAPI) ListOperations(ctx context.Context, arg params.OperationQueryArgs) (params.OperationResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.OperationResults{}, errors.Capture(err)
	}

	args := operation.QueryArgs{
		Target:      makeOperationTarget(arg.Applications, arg.Machines, arg.Units),
		ActionNames: arg.ActionNames,
		Status:      arg.Status,
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
	// Map from operation tag string to all result slots (to support duplicate tags)
	tagToResults := make(map[string][]*params.OperationResult)

	operationIds := make([]string, 0, len(arg.Entities))
	for i, entity := range arg.Entities {
		operationTag, err := names.ParseOperationTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		tagStr := operationTag.String()
		tagToResults[tagStr] = append(tagToResults[tagStr], &results.Results[i])
		operationIds = append(operationIds, operationTag.Id())
	}

	if len(operationIds) == 0 {
		return results, nil
	}

	result, err := a.operationService.GetOperationsByIDs(ctx, operationIds)
	if err != nil {
		return params.OperationResults{}, errors.Capture(err)
	}
	facadeResult := toOperationResults(result)
	// Fill in all matching slots for each returned operation
	for _, r := range facadeResult.Results {
		slots := tagToResults[r.OperationTag]
		if len(slots) == 0 {
			return params.OperationResults{}, errors.Errorf("unexpected result for %q", r.OperationTag)
		}
		for _, slot := range slots {
			*slot = r
		}
	}
	// Mark any slots that did not receive a result as NotFound
	for tag, slots := range tagToResults {
		for _, slot := range slots {
			if slot.OperationTag == "" && slot.Error == nil {
				slot.Error = apiservererrors.ServerError(jujuerrors.NotFoundf("missing result for %q", tag))
			}
		}
	}
	return results, nil
}
