// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

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
	operationID, err := a.model.EnqueueOperation(summary)
	if err != nil {
		return "", params.ActionResults{}, errors.Annotate(err, "creating operation for actions")
	}

	tagToActionReceiver := common.TagToActionReceiverFn(a.state.FindEntity)
	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Actions))}
	for i, action := range arg.Actions {
		currentResult := &response.Results[i]
		actionReceiver := action.Receiver
		if strings.HasSuffix(actionReceiver, "leader") {
			app := strings.Split(actionReceiver, "/")[0]
			receiverName, err := getLeader(app)
			if err != nil {
				currentResult.Error = common.ServerError(err)
				continue
			}
			actionReceiver = names.NewUnitTag(receiverName).String()
		}
		receiver, err := tagToActionReceiver(actionReceiver)
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		enqueued, err := receiver.AddAction(operationID, action.Name, action.Parameters)
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}

		response.Results[i] = common.MakeActionResult(receiver.Tag(), enqueued, false)
	}
	return operationID, response, nil
}

// Operations fetches the called functions (actions) for specified apps/units.
func (a *ActionAPI) Operations(arg params.OperationQueryArgs) (params.ActionResults, error) {
	if err := a.checkCanRead(); err != nil {
		return params.ActionResults{}, errors.Trace(err)
	}

	unitTags := set.NewStrings()
	for _, name := range arg.Units {
		unitTags.Add(names.NewUnitTag(name).String())
	}
	appNames := arg.Applications
	if len(appNames) == 0 && unitTags.Size() == 0 {
		apps, err := a.state.AllApplications()
		if err != nil {
			return params.ActionResults{}, errors.Trace(err)
		}
		for _, a := range apps {
			appNames = append(appNames, a.Name())
		}
	}
	for _, aName := range appNames {
		app, err := a.state.Application(aName)
		if err != nil {
			return params.ActionResults{}, errors.Trace(err)
		}
		units, err := app.AllUnits()
		if err != nil {
			return params.ActionResults{}, errors.Trace(err)
		}
		for _, u := range units {
			unitTags.Add(u.Tag().String())
		}
	}

	var entities params.Entities
	for _, unitTag := range unitTags.SortedValues() {
		entities.Entities = append(entities.Entities, params.Entity{Tag: unitTag})
	}

	statusSet := set.NewStrings(arg.Status...)
	if statusSet.Size() == 0 {
		statusSet = set.NewStrings(params.ActionPending, params.ActionRunning, params.ActionCompleted)
	}
	var extractorFuncs []extractorFn
	for _, status := range statusSet.SortedValues() {
		switch status {
		case params.ActionPending:
			extractorFuncs = append(extractorFuncs, pendingActions)
		case params.ActionRunning:
			extractorFuncs = append(extractorFuncs, runningActions)
		case params.ActionCompleted:
			extractorFuncs = append(extractorFuncs, completedActions)
		}
	}

	byReceivers, err := a.internalList(entities, combine(extractorFuncs...), false)
	if err != nil {
		return params.ActionResults{}, errors.Trace(err)
	}

	nameMatches := func(name string, filter []string) bool {
		if len(filter) == 0 {
			return true
		}
		for _, f := range filter {
			if f == name {
				return true
			}
		}
		return false
	}

	var result params.ActionResults
	for _, actions := range byReceivers.Actions {
		if actions.Error != nil {
			return params.ActionResults{}, errors.Trace(actions.Error)
		}
		for _, ar := range actions.Actions {
			if nameMatches(ar.Action.Name, arg.FunctionNames) {
				result.Results = append(result.Results, ar)
			}
		}
	}
	return result, nil
}
