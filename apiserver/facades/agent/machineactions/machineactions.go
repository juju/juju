// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	coremachine "github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/operation"
	operationerrors "github.com/juju/juju/domain/operation/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// OperationService provides access to operations and tasks.
type OperationService interface {
	// GetPendingTaskByTaskID return a struct containing the data required to
	// run a task. The task must have a status of pending.
	// Returns TaskNotPending if the task exists but does not have
	// a pending status.
	GetPendingTaskByTaskID(ctx context.Context, id string) (operation.TaskArgs, error)

	// GetReceiverFromTaskID return a receiver string for the task identified.
	// The string should satisfy the ActionReceiverTag type.
	GetReceiverFromTaskID(ctx context.Context, id string) (string, error)

	// FinishTask saves the result of a completed Task.
	FinishTask(context.Context, operation.CompletedTaskResult) error

	// StartTask marks a task as running and logs the time it was started.
	StartTask(ctx context.Context, id string) error

	// WatchMachineTaskNotifications returns a StringsWatcher that emits task
	// ids for tasks targeted at the provided machine.
	// NOTE: This watcher will emit events for tasks changing their statuses to
	// PENDING only.
	WatchMachineTaskNotifications(ctx context.Context, machineName coremachine.Name) (watcher.StringsWatcher, error)

	// GetMachineTaskIDsWithStatus retrieves all task IDs for a machine with the specified status.
	GetMachineTaskIDsWithStatus(ctx context.Context, name coremachine.Name, running corestatus.Status) ([]string, error)
}

// Facade implements the machineactions interface and is the concrete
// implementation of the api end point.
type Facade struct {
	watcherRegistry facade.WatcherRegistry
	accessMachine   common.AuthFunc

	operationService OperationService
}

// NewFacade creates a new server-side machineactions API end point.
func NewFacade(
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
	operationService OperationService,
) (*Facade, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}
	return &Facade{
		watcherRegistry:  watcherRegistry,
		accessMachine:    authorizer.AuthOwner,
		operationService: operationService,
	}, nil
}

// Actions returns the Actions by Tags passed and ensures that the machine asking
// for them is the machine that has the actions.
func (f *Facade) Actions(ctx context.Context, args params.Entities) params.ActionResults {
	results := params.ActionResults{
		Results: make([]params.ActionResult, len(args.Entities)),
	}

	for i, arg := range args.Entities {
		taskID, err := f.authTaskID(ctx, arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		task, err := f.operationService.GetPendingTaskByTaskID(ctx, taskID)
		if errors.Is(err, operationerrors.TaskNotPending) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrActionNotAvailable)
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Action = &params.Action{
			Name:           task.ActionName,
			Parameters:     task.Parameters,
			Parallel:       ptr(task.IsParallel),
			ExecutionGroup: nilZeroPtr(task.ExecutionGroup),
		}
	}

	return results
}

// BeginActions marks the actions represented by the passed in Tags as running.
func (f *Facade) BeginActions(ctx context.Context, args params.Entities) params.ErrorResults {
	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Entities))}

	for i, arg := range args.Entities {
		actionID, err := f.authTaskID(ctx, arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		err = f.operationService.StartTask(ctx, actionID)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}

	return results
}

// authTaskID tests the task receiver for the task ID to authenticate against,
// returns the task ID if successful.
func (f *Facade) authTaskID(ctx context.Context, tagStr string) (string, error) {
	actionTag, err := names.ParseActionTag(tagStr)
	if err != nil {
		return "", err
	}
	receiverStr, err := f.operationService.GetReceiverFromTaskID(ctx, actionTag.Id())
	if err != nil {
		return "", err
	}
	receiverTag, err := names.ActionReceiverTag(receiverStr)
	if err != nil {
		return "", err
	}
	if !f.accessMachine(receiverTag) {
		return "", apiservererrors.ErrPerm
	}
	return actionTag.Id(), nil
}

// FinishActions saves the result of a completed Action and sets
// its status to completed.
func (f *Facade) FinishActions(ctx context.Context, args params.ActionExecutionResults) params.ErrorResults {
	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Results))}

	for i, arg := range args.Results {
		taskID, err := f.authTaskID(ctx, arg.ActionTag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		taskResultArg := operation.CompletedTaskResult{
			TaskID:  taskID,
			Status:  arg.Status,
			Results: arg.Results,
			Message: arg.Message,
		}
		err = f.operationService.FinishTask(ctx, taskResultArg)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}

	return results
}

// WatchActionNotifications returns a StringsWatcher for observing
// incoming action calls to a machine.
func (f *Facade) WatchActionNotifications(ctx context.Context, args params.Entities) params.StringsWatchResults {
	results := make([]params.StringsWatchResult, len(args.Entities))

	for i, entity := range args.Entities {
		result := &results[i]

		machineTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}
		if !f.accessMachine(machineTag) {
			result.Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		machineName := machineTag.Id()

		watcher, err := f.operationService.WatchMachineTaskNotifications(ctx, coremachine.Name(machineName))
		if errors.Is(err, machineerrors.MachineNotFound) {
			result.Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", machineName)
			continue
		} else if err != nil {
			result.Error = apiservererrors.ServerError(err)
			continue
		}

		id, _, err := internal.EnsureRegisterWatcher(ctx, f.watcherRegistry, watcher)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
			continue
		}

		result.StringsWatcherId = id
	}
	return params.StringsWatchResults{Results: results}
}

// RunningActions lists the actions running for the entities passed in.
// If we end up needing more than ListRunning at some point we could follow/abstract
// what's done in the client actions package.
//
// Note(gfouillet): this implementation is minimal to fulfill requirements for
// the client method RunningActions in `api/agent/machineactions/machineactions.go`
// and its only user in the machineactions worker
// in the SetUp method in `internal/worker/machineactions/worker.go` which
// only query for machine as entities and only use the action ID (eg TaskID in 4.x)
func (f *Facade) RunningActions(ctx context.Context, args params.Entities) params.ActionsByReceivers {
	canAccess := f.accessMachine

	response := params.ActionsByReceivers{
		Actions: make([]params.ActionsByReceiver, len(args.Entities)),
	}

	for i, entity := range args.Entities {
		currentResult := &response.Actions[i]
		receiverTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}
		currentResult.Receiver = receiverTag.String()

		if !canAccess(receiverTag) {
			currentResult.Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		taskIDs, err := f.operationService.GetMachineTaskIDsWithStatus(
			ctx,
			coremachine.Name(receiverTag.Id()),
			corestatus.Running,
		)
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(err)
			continue
		}
		currentResult.Actions = transform.Slice(taskIDs, func(id string) params.ActionResult {
			return params.ActionResult{
				Action: &params.Action{
					Tag: names.NewActionTag(id).String(),
				},
			}
		})
	}

	return response
}

func ptr[T any](v T) *T {
	return &v
}

func nilZeroPtr[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}
