// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// ParamsActionExecutionResultsToStateActionResults does exactly what
// the name implies.
func ParamsActionExecutionResultsToStateActionResults(arg params.ActionExecutionResult) (state.ActionResults, error) {
	var status state.ActionStatus
	switch arg.Status {
	case params.ActionCancelled:
		status = state.ActionCancelled
	case params.ActionCompleted:
		status = state.ActionCompleted
	case params.ActionFailed:
		status = state.ActionFailed
	case params.ActionPending:
		status = state.ActionPending
	default:
		return state.ActionResults{}, errors.Errorf("unrecognized action status '%s'", arg.Status)
	}
	return state.ActionResults{
		Status:  status,
		Results: arg.Results,
		Message: arg.Message,
	}, nil
}

// TagToActionReceiver takes a tag string and tries to convert it to an
// ActionReceiver. It needs a findEntity function passed in that can search for the tags in state.
func TagToActionReceiverFn(findEntity func(names.Tag) (state.Entity, error)) func(tag string) (state.ActionReceiver, error) {
	return func(tag string) (state.ActionReceiver, error) {
		receiverTag, err := names.ParseTag(tag)
		if err != nil {
			return nil, ErrBadId
		}
		entity, err := findEntity(receiverTag)
		if err != nil {
			return nil, ErrBadId
		}
		receiver, ok := entity.(state.ActionReceiver)
		if !ok {
			return nil, ErrBadId
		}
		return receiver, nil
	}
}

// AuthAndActionFromTagFn takes in an authorizer function and a function that can fetch action by tags from state
// and returns a function that can fetch an action from state by id and check the authorization.
func AuthAndActionFromTagFn(canAccess AuthFunc, getActionByTag func(names.ActionTag) (state.Action, error)) func(string) (state.Action, error) {
	return func(tag string) (state.Action, error) {
		actionTag, err := names.ParseActionTag(tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		action, err := getActionByTag(actionTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		receiverTag, err := names.ActionReceiverTag(action.Receiver())
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !canAccess(receiverTag) {
			return nil, ErrPerm
		}
		return action, nil
	}
}

// BeginActions is a helper function currently used by the uniter and by machineactions
// It needs an actionFn that can fetch an action from state using it's id.
// It is usually created by AuthAndActionFromTagFn
func BeginActions(args params.Entities, actionFn func(string) (state.Action, error)) params.ErrorResults {
	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Entities))}

	for i, arg := range args.Entities {
		action, err := actionFn(arg.Tag)
		if err != nil {
			results.Results[i].Error = ServerError(err)
			continue
		}

		_, err = action.Begin()
		if err != nil {
			results.Results[i].Error = ServerError(err)
			continue
		}
	}

	return results
}

// FinishActions is a helper function currently used by the uniter and by machineactions
// It needs an actionFn that can fetch an action from state using it's id.
// It is usually created by AuthAndActionFromTagFn
func FinishActions(args params.ActionExecutionResults, actionFn func(string) (state.Action, error)) params.ErrorResults {
	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Results))}

	for i, arg := range args.Results {
		action, err := actionFn(arg.ActionTag)
		if err != nil {
			results.Results[i].Error = ServerError(err)
			continue
		}
		actionResults, err := ParamsActionExecutionResultsToStateActionResults(arg)
		if err != nil {
			results.Results[i].Error = ServerError(err)
			continue
		}

		_, err = action.Finish(actionResults)
		if err != nil {
			results.Results[i].Error = ServerError(err)
			continue
		}
	}

	return results
}

// Actions is a helper function currently used by the uniter and by machineactions
// It needs an actionFn that can fetch an action from state using it's id.
// It is usually created by AuthAndActionFromTagFn
func Actions(args params.Entities, actionFn func(string) (state.Action, error)) params.ActionResults {
	results := params.ActionResults{
		Results: make([]params.ActionResult, len(args.Entities)),
	}

	for i, arg := range args.Entities {
		action, err := actionFn(arg.Tag)
		if err != nil {
			results.Results[i].Error = ServerError(err)
			continue
		}
		if action.Status() != state.ActionPending {
			results.Results[i].Error = ServerError(ErrActionNotAvailable)
			continue
		}
		results.Results[i].Action = &params.Action{
			Name:       action.Name(),
			Parameters: action.Parameters(),
		}
	}

	return results
}

// WatchOneActionReceiverNotifications is a helper function currently used by the uniter and by machineactions
// to create a watcher for one receiver. It needs a tagToActionReceiver function and a registerFunc to register
// resources.
func WatchOneActionReceiverNotifications(tagToActionReceiver func(tag string) (state.ActionReceiver, error), registerFunc func(r Resource) string) func(names.Tag) (params.StringsWatchResult, error) {
	return func(tag names.Tag) (params.StringsWatchResult, error) {
		nothing := params.StringsWatchResult{}
		receiver, err := tagToActionReceiver(tag.String())
		if err != nil {
			return nothing, err
		}
		watch := receiver.WatchActionNotifications()

		if changes, ok := <-watch.Changes(); ok {
			return params.StringsWatchResult{
				StringsWatcherId: registerFunc(watch),
				Changes:          changes,
			}, nil
		}
		return nothing, watcher.EnsureErr(watch)
	}
}

// WatchActionNotifications is a helper function currently used by the uniter and by machineactions
// to create watchers. The canAccess function is passed in by the respective caller to provide authorization.
// watchOne is usually a function created by WatchOneActionReceiverNotifications
func WatchActionNotifications(args params.Entities, canAccess AuthFunc, watchOne func(names.Tag) (params.StringsWatchResult, error)) params.StringsWatchResults {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}

	for i, entity := range args.Entities {
		tag, err := names.ActionReceiverFromTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		err = ErrPerm
		if canAccess(tag) {
			result.Results[i], err = watchOne(tag)
		}
		result.Results[i].Error = ServerError(err)
	}

	return result
}

// GetActionsFn declares the function type that returns a slice of
// state.Action and error, used to curry specific list functions.
type GetActionsFn func() ([]state.Action, error)

// ConvertActions takes a generic getActionsFn to obtain a slice
// of state.Action and then converts them to the API slice of
// params.ActionResult.
func ConvertActions(ar state.ActionReceiver, fn GetActionsFn) ([]params.ActionResult, error) {
	items := []params.ActionResult{}
	actions, err := fn()
	if err != nil {
		return items, err
	}
	for _, action := range actions {
		if action == nil {
			continue
		}
		items = append(items, MakeActionResult(ar.Tag(), action))
	}
	return items, nil
}

// MakeActionResult does the actual type conversion from state.Action
// to params.ActionResult.
func MakeActionResult(actionReceiverTag names.Tag, action state.Action) params.ActionResult {
	output, message := action.Results()
	return params.ActionResult{
		Action: &params.Action{
			Receiver:   actionReceiverTag.String(),
			Tag:        action.ActionTag().String(),
			Name:       action.Name(),
			Parameters: action.Parameters(),
		},
		Status:    string(action.Status()),
		Message:   message,
		Output:    output,
		Enqueued:  action.Enqueued(),
		Started:   action.Started(),
		Completed: action.Completed(),
	}
}
