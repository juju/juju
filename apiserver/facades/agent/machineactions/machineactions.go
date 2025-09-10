// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/rpc/params"
)

// OperationService provides access to operations and tasks.
type OperationService interface {
	// StartTask marks a task as running and logs the time it was started.
	StartTask(ctx context.Context, id string) error

	// FinishTask saves the result of a completed Task.
	FinishTask(context.Context, operation.CompletedTaskResult) error

	// ReceiverFromTask return a receiver string for the task identified.
	// The string should satisfy the ActionReceiverTag type.
	ReceiverFromTask(ctx context.Context, id string) (string, error)

	// WatchMachineTaskNotifications returns a StringsWatcher that emits task
	// ids for tasks targeted at the provided machine.
	// This watcher emits all tasks no matter their status.
	WatchMachineTaskNotifications(ctx context.Context, machineName coremachine.Name) (watcher.StringsWatcher, error)
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
) (*Facade, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}
	return &Facade{
		watcherRegistry: watcherRegistry,
		accessMachine:   authorizer.AuthOwner,
	}, nil
}

// Actions returns the Actions by Tags passed and ensures that the machine asking
// for them is the machine that has the actions
func (f *Facade) Actions(ctx context.Context, args params.Entities) params.ActionResults {
	return params.ActionResults{}
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
	receiverStr, err := f.operationService.ReceiverFromTask(ctx, actionTag.Id())
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

		machineTag := names.NewMachineTag(entity.Tag)
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
func (f *Facade) RunningActions(ctx context.Context, args params.Entities) params.ActionsByReceivers {
	return params.ActionsByReceivers{
		Actions: make([]params.ActionsByReceiver, len(args.Entities)),
	}
}
