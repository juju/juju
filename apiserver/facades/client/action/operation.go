// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// EnqueueOperation takes a list of Actions and queues them up to be executed as
// an operation, each action running as a task on the designated ActionReceiver.
// We return the ID of the overall operation and each individual task.
func (a *ActionAPI) EnqueueOperation(arg params.Actions) (params.EnqueuedActions, error) {
	operationId, actionResults, err := a.enqueue(arg)
	if err != nil {
		return params.EnqueuedActions{}, err
	}
	results := params.EnqueuedActions{
		OperationTag: names.NewOperationTag(operationId).String(),
		Actions:      actionResults.Results,
	}
	return results, nil
}

func (a *ActionAPI) enqueue(arg params.Actions) (string, params.ActionResults, error) {
	if err := a.checkCanWrite(); err != nil {
		return "", params.ActionResults{}, errors.Trace(err)
	}

	var leaders map[string]string
	getLeader := func(appName string) (string, error) {
		if leaders == nil {
			var err error
			leaders, err = a.state.ApplicationLeaders()
			if err != nil {
				return "", err
			}
		}
		if leader, ok := leaders[appName]; ok {
			return leader, nil
		}
		return "", errors.Errorf("could not determine leader for %q", appName)
	}

	var operationName string
	var receivers []string
	for _, a := range arg.Actions {
		if a.Receiver != "" {
			receivers = append(receivers, a.Receiver)
		}
		if operationName == "" {
			operationName = a.Name
			continue
		}
		if operationName != a.Name {
			operationName = "multiple actions"
		}
	}
	summary := fmt.Sprintf("%v run on %v", operationName, strings.Join(receivers, ","))
	operationID, err := a.model.EnqueueOperation(summary, len(receivers))
	if err != nil {
		return "", params.ActionResults{}, errors.Annotate(err, "creating operation for actions")
	}

	tagToActionReceiver := a.tagToActionReceiverFn(a.state.FindEntity)
	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Actions))}
	for i, action := range arg.Actions {
		actionReceiver := action.Receiver
		var (
			actionErr    error
			enqueued     state.Action
			receiver     state.ActionReceiver
			receiverName string
		)
		if strings.HasSuffix(actionReceiver, "leader") {
			app := strings.Split(actionReceiver, "/")[0]
			receiverName, actionErr = getLeader(app)
			if actionErr != nil {
				response.Results[i].Error = apiservererrors.ServerError(actionErr)
				continue
			}
			actionReceiver = names.NewUnitTag(receiverName).String()
		}
		receiver, actionErr = tagToActionReceiver(actionReceiver)
		if actionErr != nil {
			response.Results[i].Error = apiservererrors.ServerError(actionErr)
			continue
		}
		enqueued, actionErr = a.model.AddAction(receiver, operationID, action.Name, action.Parameters, action.Parallel, action.ExecutionGroup)
		if actionErr != nil {
			response.Results[i].Error = apiservererrors.ServerError(actionErr)
			continue
		}

		response.Results[i] = common.MakeActionResult(receiver.Tag(), enqueued)
		continue
	}

	err = a.handleFailedActionEnqueuing(operationID, response, len(arg.Actions))
	return operationID, response, errors.Trace(err)
}

func (a *ActionAPI) handleFailedActionEnqueuing(operationID string, response params.ActionResults, argCount int) error {
	failMessages := make([]string, 0)
	for _, res := range response.Results {
		if res.Error != nil {
			failMessages = append(failMessages, res.Error.Error())
		}
	}
	if len(failMessages) == 0 {
		return nil
	}
	startedCount := argCount - len(failMessages)
	failMessage := fmt.Sprintf("error(s) enqueueing action(s): %s", strings.Join(failMessages, ", "))
	return a.model.FailOperationEnqueuing(operationID, failMessage, startedCount)
}

// ListOperations fetches the called actions for specified apps/units.
func (a *ActionAPI) ListOperations(arg params.OperationQueryArgs) (params.OperationResults, error) {
	if err := a.checkCanRead(); err != nil {
		return params.OperationResults{}, errors.Trace(err)
	}

	var receiverTags []names.Tag
	for _, name := range arg.Units {
		receiverTags = append(receiverTags, names.NewUnitTag(name))
	}
	for _, id := range arg.Machines {
		receiverTags = append(receiverTags, names.NewMachineTag(id))
	}
	appNames := arg.Applications
	if len(arg.ActionNames) == 0 && len(appNames) == 0 && len(receiverTags) == 0 {
		apps, err := a.state.AllApplications()
		if err != nil {
			return params.OperationResults{}, errors.Trace(err)
		}
		for _, a := range apps {
			appNames = append(appNames, a.Name())
		}
	}
	for _, aName := range appNames {
		app, err := a.state.Application(aName)
		if err != nil {
			return params.OperationResults{}, errors.Trace(err)
		}
		units, err := app.AllUnits()
		if err != nil {
			return params.OperationResults{}, errors.Trace(err)
		}
		for _, u := range units {
			receiverTags = append(receiverTags, u.Tag())
		}
	}

	status := set.NewStrings(arg.Status...)
	actionStatus := make([]state.ActionStatus, len(status))
	for i, s := range status.Values() {
		actionStatus[i] = state.ActionStatus(s)

	}
	limit := 0
	if arg.Limit != nil {
		limit = *arg.Limit
	}
	offset := 0
	if arg.Offset != nil {
		offset = *arg.Offset
	}
	summaryResults, truncated, err := a.model.ListOperations(arg.ActionNames, receiverTags, actionStatus, offset, limit)
	if err != nil {
		return params.OperationResults{}, errors.Trace(err)
	}

	result := params.OperationResults{
		Truncated: truncated,
		Results:   make([]params.OperationResult, len(summaryResults)),
	}
	for i, r := range summaryResults {
		result.Results[i] = params.OperationResult{
			OperationTag: r.Operation.Tag().String(),
			Summary:      r.Operation.Summary(),
			Fail:         r.Operation.Fail(),
			Enqueued:     r.Operation.Enqueued(),
			Started:      r.Operation.Started(),
			Completed:    r.Operation.Completed(),
			Status:       string(r.Operation.Status()),
			Actions:      make([]params.ActionResult, len(r.Actions)),
		}
		for j, a := range r.Actions {
			receiver, err := names.ActionReceiverTag(a.Receiver())
			if err == nil {
				result.Results[i].Actions[j] = common.MakeActionResult(receiver, a)
				continue
			}
			result.Results[i].Actions[j] = params.ActionResult{
				Error: apiservererrors.ServerError(errors.Errorf("unknown action receiver %q", a.Receiver())),
			}
		}
	}
	return result, nil
}

// Operations fetches the specified operation ids.
func (a *ActionAPI) Operations(arg params.Entities) (params.OperationResults, error) {
	if err := a.checkCanRead(); err != nil {
		return params.OperationResults{}, errors.Trace(err)
	}
	results := params.OperationResults{Results: make([]params.OperationResult, len(arg.Entities))}

	for i, entity := range arg.Entities {
		tag, err := names.ParseOperationTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		op, err := a.model.OperationWithActions(tag.Id())
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		results.Results[i] = params.OperationResult{
			OperationTag: op.Operation.Tag().String(),
			Summary:      op.Operation.Summary(),
			Fail:         op.Operation.Fail(),
			Enqueued:     op.Operation.Enqueued(),
			Started:      op.Operation.Started(),
			Completed:    op.Operation.Completed(),
			Status:       string(op.Operation.Status()),
			Actions:      make([]params.ActionResult, len(op.Actions)),
		}
		for j, a := range op.Actions {
			receiver, err := names.ActionReceiverTag(a.Receiver())
			if err == nil {
				results.Results[i].Actions[j] = common.MakeActionResult(receiver, a)
				continue
			}
			results.Results[i].Actions[j] = params.ActionResult{
				Error: apiservererrors.ServerError(errors.Errorf("unknown action receiver %q", a.Receiver())),
			}
		}
	}
	return results, nil
}
