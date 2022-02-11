// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.action")

// EnqueueOperation isn't on the V5 API.
func (*APIv5) EnqueueOperation(_, _ struct{}) {}

// EnqueueOperation takes a list of Actions and queues them up to be executed as
// an operation, each action running as a task on the the designated ActionReceiver.
// We return the ID of the overall operation and each individual task.
func (a *ActionAPI) EnqueueOperation(arg params.Actions) (params.EnqueuedActions, error) {
	operationId, actionResults, err := a.enqueue(arg)
	if err != nil {
		return params.EnqueuedActions{}, err
	}
	results := params.EnqueuedActions{
		OperationTag: names.NewOperationTag(operationId).String(),
		Actions:      make([]params.StringResult, len(actionResults.Results)),
	}
	for i, action := range actionResults.Results {
		results.Actions[i].Error = action.Error
		if action.Action != nil {
			results.Actions[i].Result = action.Action.Tag
		}
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

	tagToActionReceiver := common.TagToActionReceiverFn(a.state.FindEntity)
	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Actions))}
	errorRecorded := false
	for i, action := range arg.Actions {
		currentResult := &response.Results[i]
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
				currentResult.Error = apiservererrors.ServerError(actionErr)
				goto failOperation
			}
			actionReceiver = names.NewUnitTag(receiverName).String()
		}
		receiver, actionErr = tagToActionReceiver(actionReceiver)
		if actionErr != nil {
			currentResult.Error = apiservererrors.ServerError(actionErr)
			goto failOperation
		}
		enqueued, actionErr = a.model.AddAction(receiver, operationID, action.Name, action.Parameters)
		if actionErr != nil {
			currentResult.Error = apiservererrors.ServerError(actionErr)
			goto failOperation
		}
		response.Results[i] = common.MakeActionResult(receiver.Tag(), enqueued, false)
		continue

	failOperation:
		// If we failed to enqueue the action, record the error on the operation.
		if !errorRecorded {
			errorRecorded = true
			err := a.model.FailOperation(operationID, actionErr)
			logger.Errorf("unable to log the error on the operation: %v", err)
		}
		currentResult.Error = apiservererrors.ServerError(actionErr)
	}
	return operationID, response, nil
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
				result.Results[i].Actions[j] = common.MakeActionResult(receiver, a, false)
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
			receiver := names.NewUnitTag(a.Receiver())
			results.Results[i].Actions[j] = common.MakeActionResult(receiver, a, false)
		}
	}
	return results, nil
}
