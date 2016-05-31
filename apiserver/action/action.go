// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("Action", 2, NewActionAPI)
}

// ActionAPI implements the client API for interacting with Actions
type ActionAPI struct {
	state      *state.State
	resources  *common.Resources
	authorizer common.Authorizer
	check      *common.BlockChecker
}

// NewActionAPI returns an initialized ActionAPI
func NewActionAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*ActionAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &ActionAPI{
		state:      st,
		resources:  resources,
		authorizer: authorizer,
		check:      common.NewBlockChecker(st),
	}, nil
}

// Actions takes a list of ActionTags, and returns the full Action for
// each ID.
func (a *ActionAPI) Actions(arg params.Entities) (params.ActionResults, error) {
	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Entities))}
	for i, entity := range arg.Entities {
		currentResult := &response.Results[i]
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			currentResult.Error = common.ServerError(common.ErrBadId)
			continue
		}
		actionTag, ok := tag.(names.ActionTag)
		if !ok {
			currentResult.Error = common.ServerError(common.ErrBadId)
			continue
		}
		action, err := a.state.ActionByTag(actionTag)
		if err != nil {
			currentResult.Error = common.ServerError(common.ErrBadId)
			continue
		}
		receiverTag, err := names.ActionReceiverTag(action.Receiver())
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		response.Results[i] = common.MakeActionResult(receiverTag, action)
	}
	return response, nil
}

// FindActionTagsByPrefix takes a list of string prefixes and finds
// corresponding ActionTags that match that prefix.
func (a *ActionAPI) FindActionTagsByPrefix(arg params.FindTags) (params.FindTagsResults, error) {
	response := params.FindTagsResults{Matches: make(map[string][]params.Entity)}
	for _, prefix := range arg.Prefixes {
		found := a.state.FindActionTagsByPrefix(prefix)
		matches := make([]params.Entity, len(found))
		for i, tag := range found {
			matches[i] = params.Entity{Tag: tag.String()}
		}
		response.Matches[prefix] = matches
	}
	return response, nil
}

func (a *ActionAPI) FindActionsByNames(arg params.FindActionsByNames) (params.ActionsByNames, error) {
	response := params.ActionsByNames{Actions: make([]params.ActionsByName, len(arg.ActionNames))}
	for i, name := range arg.ActionNames {
		currentResult := &response.Actions[i]
		currentResult.Name = name

		actions, err := a.state.FindActionsByName(name)
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		for _, action := range actions {
			recvTag, err := names.ActionReceiverTag(action.Receiver())
			if err != nil {
				currentResult.Actions = append(currentResult.Actions, params.ActionResult{Error: common.ServerError(err)})
				continue
			}
			currentAction := common.MakeActionResult(recvTag, action)
			currentResult.Actions = append(currentResult.Actions, currentAction)
		}
	}
	return response, nil
}

// Enqueue takes a list of Actions and queues them up to be executed by
// the designated ActionReceiver, returning the params.Action for each
// enqueued Action, or an error if there was a problem enqueueing the
// Action.
func (a *ActionAPI) Enqueue(arg params.Actions) (params.ActionResults, error) {
	if err := a.check.ChangeAllowed(); err != nil {
		return params.ActionResults{}, errors.Trace(err)
	}

	tagToActionReceiver := common.TagToActionReceiverFn(a.state.FindEntity)
	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Actions))}
	for i, action := range arg.Actions {
		currentResult := &response.Results[i]
		receiver, err := tagToActionReceiver(action.Receiver)
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		enqueued, err := receiver.AddAction(action.Name, action.Parameters)
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}

		response.Results[i] = common.MakeActionResult(receiver.Tag(), enqueued)
	}
	return response, nil
}

// ListAll takes a list of Entities representing ActionReceivers and
// returns all of the Actions that have been enqueued or run by each of
// those Entities.
func (a *ActionAPI) ListAll(arg params.Entities) (params.ActionsByReceivers, error) {
	return a.internalList(arg, combine(pendingActions, runningActions, completedActions))
}

// ListPending takes a list of Entities representing ActionReceivers
// and returns all of the Actions that are enqueued for each of those
// Entities.
func (a *ActionAPI) ListPending(arg params.Entities) (params.ActionsByReceivers, error) {
	return a.internalList(arg, pendingActions)
}

// ListRunning takes a list of Entities representing ActionReceivers and
// returns all of the Actions that have are running on each of those
// Entities.
func (a *ActionAPI) ListRunning(arg params.Entities) (params.ActionsByReceivers, error) {
	return a.internalList(arg, runningActions)
}

// ListCompleted takes a list of Entities representing ActionReceivers
// and returns all of the Actions that have been run on each of those
// Entities.
func (a *ActionAPI) ListCompleted(arg params.Entities) (params.ActionsByReceivers, error) {
	return a.internalList(arg, completedActions)
}

// Cancel attempts to cancel enqueued Actions from running.
func (a *ActionAPI) Cancel(arg params.Entities) (params.ActionResults, error) {
	if err := a.check.ChangeAllowed(); err != nil {
		return params.ActionResults{}, errors.Trace(err)
	}

	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Entities))}
	for i, entity := range arg.Entities {
		currentResult := &response.Results[i]
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			currentResult.Error = common.ServerError(common.ErrBadId)
			continue
		}
		actionTag, ok := tag.(names.ActionTag)
		if !ok {
			currentResult.Error = common.ServerError(common.ErrBadId)
			continue
		}
		action, err := a.state.ActionByTag(actionTag)
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		result, err := action.Finish(state.ActionResults{Status: state.ActionCancelled, Message: "action cancelled via the API"})
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		receiverTag, err := names.ActionReceiverTag(result.Receiver())
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}

		response.Results[i] = common.MakeActionResult(receiverTag, result)
	}
	return response, nil
}

// ApplicationsCharmsActions returns a slice of charm Actions for a slice of
// services.
func (a *ActionAPI) ApplicationsCharmsActions(args params.Entities) (params.ApplicationsCharmActionsResults, error) {
	result := params.ApplicationsCharmActionsResults{Results: make([]params.ApplicationCharmActionsResult, len(args.Entities))}
	for i, entity := range args.Entities {
		currentResult := &result.Results[i]
		svcTag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			currentResult.Error = common.ServerError(common.ErrBadId)
			continue
		}
		currentResult.ApplicationTag = svcTag.String()
		svc, err := a.state.Service(svcTag.Id())
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		ch, _, err := svc.Charm()
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		currentResult.Actions = ch.Actions()
	}
	return result, nil
}

// internalList takes a list of Entities representing ActionReceivers
// and returns all of the Actions the extractorFn can get out of the
// ActionReceiver.
func (a *ActionAPI) internalList(arg params.Entities, fn extractorFn) (params.ActionsByReceivers, error) {
	tagToActionReceiver := common.TagToActionReceiverFn(a.state.FindEntity)
	response := params.ActionsByReceivers{Actions: make([]params.ActionsByReceiver, len(arg.Entities))}
	for i, entity := range arg.Entities {
		currentResult := &response.Actions[i]
		receiver, err := tagToActionReceiver(entity.Tag)
		if err != nil {
			currentResult.Error = common.ServerError(common.ErrBadId)
			continue
		}
		currentResult.Receiver = receiver.Tag().String()

		results, err := fn(receiver)
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		currentResult.Actions = results
	}
	return response, nil
}

// extractorFn is the generic signature for functions that extract
// state.Actions from an ActionReceiver, and return them as a slice of
// params.ActionResult.
type extractorFn func(state.ActionReceiver) ([]params.ActionResult, error)

// combine takes multiple extractorFn's and combines them into one
// function.
func combine(funcs ...extractorFn) extractorFn {
	return func(ar state.ActionReceiver) ([]params.ActionResult, error) {
		result := []params.ActionResult{}
		for _, fn := range funcs {
			items, err := fn(ar)
			if err != nil {
				return result, err
			}
			result = append(result, items...)
		}
		return result, nil
	}
}

// pendingActions iterates through the Actions() enqueued for an
// ActionReceiver, and converts them to a slice of params.ActionResult.
func pendingActions(ar state.ActionReceiver) ([]params.ActionResult, error) {
	return common.ConvertActions(ar, ar.PendingActions)
}

// runningActions iterates through the Actions() running on an
// ActionReceiver, and converts them to a slice of params.ActionResult.
func runningActions(ar state.ActionReceiver) ([]params.ActionResult, error) {
	return common.ConvertActions(ar, ar.RunningActions)
}

// completedActions iterates through the Actions() that have run to
// completion for an ActionReceiver, and converts them to a slice of
// params.ActionResult.
func completedActions(ar state.ActionReceiver) ([]params.ActionResult, error) {
	return common.ConvertActions(ar, ar.CompletedActions)
}
