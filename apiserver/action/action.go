// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.action")

func init() {
	common.RegisterStandardFacade("Action", 0, NewActionAPI)
}

// ActionAPI implements the client API for interacting with Actions
type ActionAPI struct {
	state      *state.State
	resources  *common.Resources
	authorizer common.Authorizer
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
		response.Results[i] = makeActionResult(receiverTag, action)
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

// Enqueue takes a list of Actions and queues them up to be executed by
// the designated ActionReceiver, returning the params.Action for each
// enqueued Action, or an error if there was a problem enqueueing the
// Action.
func (a *ActionAPI) Enqueue(arg params.Actions) (params.ActionResults, error) {
	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Actions))}
	for i, action := range arg.Actions {
		currentResult := &response.Results[i]
		receiver, err := tagToActionReceiver(a.state, action.Receiver)
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		enqueued, err := receiver.AddAction(action.Name, action.Parameters)
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}

		response.Results[i] = makeActionResult(receiver.Tag(), enqueued)
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

		response.Results[i] = makeActionResult(receiverTag, result)
	}
	return response, nil
}

// ServicesCharmActions returns a slice of charm Actions for a slice of
// services.
func (a *ActionAPI) ServicesCharmActions(args params.Entities) (params.ServicesCharmActionsResults, error) {
	result := params.ServicesCharmActionsResults{Results: make([]params.ServiceCharmActionsResult, len(args.Entities))}
	for i, entity := range args.Entities {
		currentResult := &result.Results[i]
		svcTag, err := names.ParseServiceTag(entity.Tag)
		if err != nil {
			currentResult.Error = common.ServerError(common.ErrBadId)
			continue
		}
		currentResult.ServiceTag = svcTag.String()
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
	response := params.ActionsByReceivers{Actions: make([]params.ActionsByReceiver, len(arg.Entities))}
	for i, entity := range arg.Entities {
		currentResult := &response.Actions[i]
		receiver, err := tagToActionReceiver(a.state, entity.Tag)
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

// tagToActionReceiver takes a tag string and tries to convert it to an
// ActionReceiver.
func tagToActionReceiver(st *state.State, tag string) (state.ActionReceiver, error) {
	receiverTag, err := names.ParseTag(tag)
	if err != nil {
		return nil, common.ErrBadId
	}
	entity, err := st.FindEntity(receiverTag)
	if err != nil {
		return nil, common.ErrBadId
	}
	receiver, ok := entity.(state.ActionReceiver)
	if !ok {
		return nil, common.ErrBadId
	}
	return receiver, nil
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
	return convertActions(ar, ar.PendingActions)
}

// runningActions iterates through the Actions() running on an
// ActionReceiver, and converts them to a slice of params.ActionResult.
func runningActions(ar state.ActionReceiver) ([]params.ActionResult, error) {
	return convertActions(ar, ar.RunningActions)
}

// completedActions iterates through the Actions() that have run to
// completion for an ActionReceiver, and converts them to a slice of
// params.ActionResult.
func completedActions(ar state.ActionReceiver) ([]params.ActionResult, error) {
	return convertActions(ar, ar.CompletedActions)
}

// getActionsFn declares the function type that returns a slice of
// *state.Action and error, used to curry specific list functions.
type getActionsFn func() ([]*state.Action, error)

// convertActions takes a generic getActionsFn to obtain a slice
// of *state.Action and then converts them to the API slice of
// params.ActionResult.
func convertActions(ar state.ActionReceiver, fn getActionsFn) ([]params.ActionResult, error) {
	items := []params.ActionResult{}
	actions, err := fn()
	if err != nil {
		return items, err
	}
	for _, action := range actions {
		if action == nil {
			continue
		}
		items = append(items, makeActionResult(ar.Tag(), action))
	}
	return items, nil
}

// makeActionResult does the actual type conversion from *state.Action
// to params.ActionResult.
func makeActionResult(actionReceiverTag names.Tag, action *state.Action) params.ActionResult {
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
