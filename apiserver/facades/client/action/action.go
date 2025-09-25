// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/operation"
	operationerrors "github.com/juju/juju/domain/operation/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// ApplicationService is an interface that provides access to application
// entities.
type ApplicationService interface {
	// GetCharmLocatorByApplicationName returns a CharmLocator by application name.
	// It returns an error if the charm can not be found by the name. This can also
	// be used as a cheap way to see if a charm exists without needing to load the
	// charm metadata.
	GetCharmLocatorByApplicationName(ctx context.Context, name string) (applicationcharm.CharmLocator, error)

	// GetCharmActions returns the actions for the charm using the charm name,
	// source and revision.
	//
	// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmActions(ctx context.Context, locator applicationcharm.CharmLocator) (internalcharm.Actions, error)
}

// ModelInfoService provides access to information about the model.
type ModelInfoService interface {
	// GetModelInfo returns information about the current model.
	GetModelInfo(context.Context) (coremodel.ModelInfo, error)
}

// OperationService provides access to operations (actions and execs).
type OperationService interface {
	// AddExecOperation creates an exec operation with tasks for various machines
	// and units, using the provided parameters.
	AddExecOperation(ctx context.Context, target operation.Receivers, args operation.ExecArgs) (operation.RunResult, error)

	// AddExecOperationOnAllMachines creates an exec operation with tasks based on
	// the provided parameters on all machines.
	AddExecOperationOnAllMachines(ctx context.Context, args operation.ExecArgs) (operation.RunResult, error)

	// AddActionOperation creates an action operation with tasks for various units
	// using the provided parameters.
	AddActionOperation(ctx context.Context, target []operation.ActionReceiver, args operation.TaskArgs) (operation.RunResult, error)

	// CancelTask attempts to cancel an enqueued task, identified by its
	// ID.
	CancelTask(ctx context.Context, taskID string) (operation.Task, error)

	// GetOperations returns a list of operations on specified entities, filtered by the
	// given parameters.
	GetOperations(ctx context.Context, args operation.QueryArgs) (operation.QueryResult, error)

	// GetOperationByID returns an operation by its ID.
	GetOperationByID(ctx context.Context, operationID string) (operation.OperationInfo, error)

	// GetTask returns the task identified by its ID.
	GetTask(ctx context.Context, taskID string) (operation.Task, error)

	// WatchTaskLogs starts and returns a StringsWatcher that notifies on new log
	// messages for a specified action being added. The strings are json encoded
	// action messages.
	WatchTaskLogs(ctx context.Context, taskID string) (watcher.StringsWatcher, error)
}

// ActionAPI implements the client API for interacting with Actions
type ActionAPI struct {
	modelTag        names.ModelTag
	authorizer      facade.Authorizer
	check           *common.BlockChecker
	leadership      leadership.Reader
	watcherRegistry facade.WatcherRegistry

	applicationService ApplicationService
	modelInfoService   ModelInfoService
	operationService   OperationService
}

// APIv7 provides the Action API facade for version 7.
type APIv7 struct {
	*ActionAPI
}

func newActionAPI(
	authorizer facade.Authorizer,
	getLeadershipReader func() (leadership.Reader, error),
	applicationService ApplicationService,
	blockCommandService common.BlockCommandService,
	modelInfoService ModelInfoService,
	operationService OperationService,
	modelUUID coremodel.UUID,
	watcherRegistry facade.WatcherRegistry,
) (*ActionAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	leaders, err := getLeadershipReader()
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelTag := names.NewModelTag(modelUUID.String())

	return &ActionAPI{
		modelTag:           modelTag,
		authorizer:         authorizer,
		check:              common.NewBlockChecker(blockCommandService),
		leadership:         leaders,
		watcherRegistry:    watcherRegistry,
		applicationService: applicationService,
		modelInfoService:   modelInfoService,
		operationService:   operationService,
	}, nil
}

func (a *ActionAPI) checkCanRead(ctx context.Context) error {
	return a.authorizer.HasPermission(ctx, permission.ReadAccess, a.modelTag)
}

func (a *ActionAPI) checkCanWrite(ctx context.Context) error {
	return a.authorizer.HasPermission(ctx, permission.WriteAccess, a.modelTag)
}

func (a *ActionAPI) checkCanAdmin(ctx context.Context) error {
	return a.authorizer.HasPermission(ctx, permission.AdminAccess, a.modelTag)
}

// Actions takes a list of ActionTags, and returns the full Action for
// each ID.
func (a *ActionAPI) Actions(ctx context.Context, arg params.Entities) (params.ActionResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.ActionResults{}, errors.Trace(err)
	}

	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Entities))}
	for i, entity := range arg.Entities {
		actionTag, err := names.ParseActionTag(entity.Tag)
		if err != nil {
			response.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}

		task, err := a.operationService.GetTask(ctx, actionTag.Id())
		if err != nil {
			// NOTE(nvinuesa): The returned error in this case is not correct
			// (should be NotFound for ActionNotFound for example), but since
			// the old API was already black-holing all the errors into a
			// ErrBadId, we keep it for backwards compatibility (in terms of
			// returned errors).
			response.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}

		receiverTag, err := names.ActionReceiverTag(task.Receiver)
		if err != nil {
			response.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		response.Results[i] = toActionResult(receiverTag, task.TaskInfo)
	}

	return response, nil
}

// Cancel attempts to cancel enqueued Actions from running.
func (a *ActionAPI) Cancel(ctx context.Context, arg params.Entities) (params.ActionResults, error) {
	if err := a.checkCanWrite(ctx); err != nil {
		return params.ActionResults{}, errors.Trace(err)
	}

	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Entities))}

	for i, entity := range arg.Entities {
		actionTag, err := names.ParseActionTag(entity.Tag)
		if err != nil {
			response.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}

		cancelledAction, err := a.operationService.CancelTask(ctx, actionTag.Id())
		if internalerrors.Is(err, operationerrors.TaskNotFound) {
			response.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("action %s", actionTag.Id()))
			continue
		}
		if err != nil {
			response.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		receiverTag, err := names.ActionReceiverTag(cancelledAction.Receiver)
		if err != nil {
			response.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		response.Results[i] = toActionResult(receiverTag, cancelledAction.TaskInfo)
	}
	return response, nil
}

// ApplicationsCharmsActions returns a slice of charm Actions for a slice of
// services.
func (a *ActionAPI) ApplicationsCharmsActions(ctx context.Context, args params.Entities) (params.ApplicationsCharmActionsResults, error) {
	result := params.ApplicationsCharmActionsResults{Results: make([]params.ApplicationCharmActionsResult, len(args.Entities))}
	if err := a.checkCanWrite(ctx); err != nil {
		return result, errors.Trace(err)
	}

	for i, entity := range args.Entities {
		currentResult := &result.Results[i]
		svcTag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}
		currentResult.ApplicationTag = svcTag.String()

		locator, err := a.applicationService.GetCharmLocatorByApplicationName(ctx, svcTag.Id())
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			currentResult.Error = apiservererrors.ServerError(errors.NotFoundf("application %q", svcTag.Id()))
			continue
		} else if err != nil {
			currentResult.Error = apiservererrors.ServerError(err)
			continue
		}

		actions, err := a.applicationService.GetCharmActions(ctx, locator)
		if errors.Is(err, applicationerrors.CharmNotFound) {
			currentResult.Error = apiservererrors.ServerError(errors.NotFoundf("charm %q", locator))
			continue
		} else if err != nil {
			currentResult.Error = apiservererrors.ServerError(err)
			continue
		}

		charmActions := make(map[string]params.ActionSpec)
		for key, value := range actions.ActionSpecs {
			charmActions[key] = params.ActionSpec{
				Description: value.Description,
				Params:      value.Params,
			}
		}
		currentResult.Actions = charmActions
	}
	return result, nil
}

// WatchActionsProgress creates a watcher that reports on action log messages.
func (api *ActionAPI) WatchActionsProgress(ctx context.Context, actions params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(actions.Entities)),
	}
	for i, arg := range actions.Entities {
		actionTag, err := names.ParseActionTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		w, err := api.operationService.WatchTaskLogs(ctx, actionTag.Id())
		if errors.Is(err, operationerrors.TaskNotFound) {
			results.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("action %q", actionTag.Id()))
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		id, initial, err := internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, w)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		results.Results[i].StringsWatcherId = id
		results.Results[i].Changes = initial
	}
	return results, nil
}

// makeOperationReceivers creates a Receivers from the given application names, machine names and unit names.
func makeOperationReceivers(applicationNames []string, machineNames []string, unitNames []string) operation.Receivers {
	return operation.Receivers{
		Applications: applicationNames,
		Machines:     transform.Slice(machineNames, as[machine.Name]),
		Units:        transform.Slice(unitNames, as[unit.Name]),
	}
}

// toOperationResults converts an operation.QueryResult to a params.OperationResults.
func toOperationResults(result operation.QueryResult) params.OperationResults {
	return params.OperationResults{
		Results:   transform.Slice(result.Operations, toOperationResult),
		Truncated: result.Truncated,
	}
}

// toOperationResult converts an operation.OperationInfo to a params.OperationResult.
func toOperationResult(op operation.OperationInfo) params.OperationResult {
	machineResult := transform.Slice(op.Machines, func(f operation.MachineTaskResult) params.ActionResult {
		result := toActionResult(names.NewMachineTag(f.ReceiverName.String()), f.TaskInfo)
		return result
	})
	unitResults := transform.Slice(op.Units, func(f operation.UnitTaskResult) params.ActionResult {
		result := toActionResult(names.NewUnitTag(f.ReceiverName.String()), f.TaskInfo)
		return result
	})
	return params.OperationResult{
		OperationTag: names.NewOperationTag(op.OperationID).String(),
		Summary:      op.Summary,
		Fail:         op.Fail,
		Enqueued:     op.Enqueued,
		Started:      op.Started,
		Completed:    op.Completed,
		Status:       op.Status.String(),
		Actions:      append(machineResult, unitResults...),
		Error:        apiservererrors.ServerError(op.Error),
	}
}

// toActionResult converts an operation.TaskInfo to a params.ActionResult.
func toActionResult(receiver names.Tag, info operation.TaskInfo) params.ActionResult {
	var logs []params.ActionMessage
	if len(info.Log) > 0 {
		logs = transform.Slice(info.Log, func(l operation.TaskLog) params.ActionMessage {
			return params.ActionMessage{Timestamp: l.Timestamp, Message: l.Message}
		})
	}
	return params.ActionResult{
		Action: &params.Action{
			Receiver:       receiver.String(),
			Tag:            names.NewActionTag(info.ID).String(),
			Name:           info.ActionName,
			Parameters:     info.Parameters,
			Parallel:       &info.IsParallel,
			ExecutionGroup: info.ExecutionGroup,
		},
		Enqueued:  info.Enqueued,
		Started:   info.Started,
		Completed: info.Completed,
		Status:    info.Status.String(),
		Message:   info.Message,
		Log:       logs,
		Output:    info.Output,
		Error:     apiservererrors.ServerError(info.Error),
	}
}

// as casts a string to a type T.
func as[T ~string](s string) T {
	return T(s)
}

// zeroNilPtr returns the zero value of T if v is nil, otherwise returns *v.
func zeroNilPtr[T comparable](v *T) T {
	var zero T
	if v == nil {
		return zero
	}
	return *v
}
