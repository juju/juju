// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.actions")

func init() {
	common.RegisterStandardFacade("Actions", 0, NewActionsAPI)
}

// ActionsAPI implements the client API for interacting with Actions
type ActionsAPI struct {
	state      *state.State
	resources  *common.Resources
	authorizer common.Authorizer
}

// NewActionsAPI returns an initialized ActionsAPI
func NewActionsAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*ActionsAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &ActionsAPI{
		state:      st,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// Enqueue takes a list of Actions and queues them up to be executed by
// the designated ActionReceiver, returning the params.Action for each
// queued Action, or an error if there was a problem queueing up the
// Action.
func (a *ActionsAPI) Enqueue(arg params.Actions) (params.ActionResults, error) {
	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Actions))}
	// TODO(jcw4) authorization checks
	for i, action := range arg.Actions {
		current := &response.Results[i]

		if action.Receiver == nil {
			current.Error = common.ServerError(common.ErrBadId)
			continue
		}

		receiver, err := tagToActionReceiver(a.state, action.Receiver)
		if err != nil {
			current.Error = common.ServerError(err)
			continue
		}

		queued, err := receiver.AddAction(action.Name, action.Parameters)
		if err != nil {
			current.Error = common.ServerError(err)
			continue
		}
		current.Action = &params.Action{
			Receiver:   receiver.Tag(),
			Tag:        queued.ActionTag(),
			Name:       queued.Name(),
			Parameters: queued.Parameters(),
		}
		current.Status = string(state.ActionPending)
	}
	return response, nil
}

// ListAll takes a list of Tags representing ActionReceivers and returns
// all of the Actions that have been queued or run by each of those
// Entities.
func (a *ActionsAPI) ListAll(arg params.Tags) (params.ActionsByReceivers, error) {
	return a.internalList(arg, combine(actionReceiverToActions, actionReceiverToActionResults))
}

// ListPending takes a list of Tags representing ActionReceivers
// and returns all of the Actions that are queued for each of those
// Entities.
func (a *ActionsAPI) ListPending(arg params.Tags) (params.ActionsByReceivers, error) {
	return a.internalList(arg, actionReceiverToActions)
}

// ListCompleted takes a list of Tags representing ActionReceivers
// and returns all of the Actions that have been run on each of those
// Entities.
func (a *ActionsAPI) ListCompleted(arg params.Tags) (params.ActionsByReceivers, error) {
	return a.internalList(arg, actionReceiverToActionResults)
}

// Cancel attempts to cancel queued up Actions from running.
func (a *ActionsAPI) Cancel(arg params.ActionTags) (params.ActionResults, error) {
	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Actions))}
	// TODO(jcw4) authorization checks
	for i, tag := range arg.Actions {
		current := &response.Results[i]
		receiver, err := tagToActionReceiver(a.state, tag.PrefixTag())
		if err != nil {
			current.Error = common.ServerError(err)
			continue
		}

		action, err := a.state.ActionByTag(tag)
		if err != nil {
			current.Error = common.ServerError(err)
			continue
		}

		result, err := receiver.CancelAction(action)
		if err != nil {
			current.Error = common.ServerError(err)
			continue
		}

		current.Action = &params.Action{
			Tag:        tag,
			Receiver:   receiver.Tag(),
			Name:       result.Name(),
			Parameters: result.Parameters(),
		}
		current.Status = string(result.Status())
		output, message := result.Results()
		current.Message = message
		current.Output = output
	}
	return response, nil
}

// ServicesCharmActions returns a slice of charm Actions for a slice of services.
func (a *ActionsAPI) ServicesCharmActions(args params.ServiceTags) (params.ServicesCharmActions, error) {
	none := params.ServicesCharmActions{}
	result := none
	for _, svcTag := range args.ServiceTags {
		newResult := params.ServiceCharmActions{ServiceTag: svcTag}
		svc, err := a.state.Service(svcTag.Id())
		if err != nil {
			newResult.Error = common.ServerError(err)
			result.Results = append(result.Results, newResult)
			continue
		}
		ch, _, err := svc.Charm()
		if err != nil {
			newResult.Error = common.ServerError(err)
			result.Results = append(result.Results, newResult)
			continue
		}
		newResult.Actions = ch.Actions()
		result.Results = append(result.Results, newResult)
	}

	return result, nil
}

// internalList takes a list of Tags representing ActionReceivers and
// returns all of the Actions the extractorFn can get out of the
// ActionReceiver.
func (a *ActionsAPI) internalList(arg params.Tags, fn extractorFn) (params.ActionsByReceivers, error) {
	response := params.ActionsByReceivers{Actions: make([]params.ActionsByReceiver, len(arg.Tags))}
	// TODO(jcw4) authorization checks
	for i, tag := range arg.Tags {
		current := &response.Actions[i]
		receiver, err := tagToActionReceiver(a.state, tag)
		if err != nil {
			current.Error = common.ServerError(common.ErrBadId)
			continue
		}
		current.Receiver = receiver.Tag()

		results, err := fn(receiver)
		if err != nil {
			current.Error = common.ServerError(err)
			continue
		}
		current.Actions = results
	}
	return response, nil
}

// tagToActionReceiver takes a names.Tag and tries to convert it to an
// ActionReceiver.
func tagToActionReceiver(st *state.State, tag names.Tag) (state.ActionReceiver, error) {
	entity, err := st.FindEntity(tag)
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
// Actions or ActionResults from an ActionReceiver, and return them as
// params.Actions.
type extractorFn func(state.ActionReceiver) ([]params.ActionResult, error)

// combine takes multiple extractorFn's and combines them into
// one function.
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

// actionReceiverToActions iterates through the Actions() queued up for
// an ActionReceiver, and converts them to a slice of params.Action.
func actionReceiverToActions(ar state.ActionReceiver) ([]params.ActionResult, error) {
	items := []params.ActionResult{}
	actions, err := ar.Actions()
	if err != nil {
		return items, err
	}
	for _, action := range actions {
		if action == nil {
			continue
		}
		items = append(items, params.ActionResult{
			Action: &params.Action{
				Receiver:   ar.Tag(),
				Tag:        action.ActionTag(),
				Name:       action.Name(),
				Parameters: action.Parameters(),
			},
			Status: string(state.ActionPending),
		})
	}
	return items, nil
}

// actionReceiverToActionResults iterates through the ActionResults()
// aqueued up for n ActionReceiver, and converts them to a slice of
// aparams.Action.
func actionReceiverToActionResults(ar state.ActionReceiver) ([]params.ActionResult, error) {
	items := []params.ActionResult{}
	results, err := ar.ActionResults()
	if err != nil {
		return items, err
	}
	for _, result := range results {
		if result == nil {
			continue
		}
		output, message := result.Results()
		items = append(items, params.ActionResult{
			Action: &params.Action{
				Receiver:   ar.Tag(),
				Tag:        result.ActionTag(),
				Name:       result.Name(),
				Parameters: result.Parameters(),
			},
			Status:  string(result.Status()),
			Message: message,
			Output:  output,
		})
	}
	return items, nil
}
