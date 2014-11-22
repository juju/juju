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

// Enqueue takes a list of Actions and queues them up to be executed by
// the designated ActionReceiver, returning the params.Action for each
// queued Action, or an error if there was a problem queueing up the
// Action.
func (a *ActionAPI) Enqueue(arg params.Actions) (params.ActionResults, error) {
	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Actions))}
	for i, action := range arg.Actions {
		current := &response.Results[i]

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
			Receiver:   receiver.Tag().String(),
			Tag:        queued.ActionTag().String(),
			Name:       queued.Name(),
			Parameters: queued.Parameters(),
		}
		current.Status = string(state.ActionPending)
	}
	return response, nil
}

// ListAll takes a list of Entities representing ActionReceivers and returns
// all of the Actions that have been queued or run by each of those
// Entities.
func (a *ActionAPI) ListAll(arg params.Entities) (params.ActionsByReceivers, error) {
	return a.internalList(arg, combine(actionReceiverToActions, actionReceiverToActionResults))
}

// ListPending takes a list of Entities representing ActionReceivers
// and returns all of the Actions that are queued for each of those
// Entities.
func (a *ActionAPI) ListPending(arg params.Entities) (params.ActionsByReceivers, error) {
	return a.internalList(arg, actionReceiverToActions)
}

// ListCompleted takes a list of Entities representing ActionReceivers
// and returns all of the Actions that have been run on each of those
// Entities.
func (a *ActionAPI) ListCompleted(arg params.Entities) (params.ActionsByReceivers, error) {
	return a.internalList(arg, actionReceiverToActionResults)
}

// Cancel attempts to cancel queued up Actions from running.
func (a *ActionAPI) Cancel(arg params.Entities) (params.ActionResults, error) {
	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Entities))}
	for i, entity := range arg.Entities {
		current := &response.Results[i]
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			current.Error = common.ServerError(common.ErrBadId)
			continue
		}
		atag, ok := tag.(names.ActionTag)
		if !ok {
			current.Error = common.ServerError(common.ErrBadId)
			continue
		}
		receiver, err := tagToActionReceiver(a.state, atag.PrefixTag().String())
		if err != nil {
			current.Error = common.ServerError(err)
			continue
		}

		action, err := a.state.ActionByTag(atag)
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
			Tag:        atag.String(),
			Receiver:   receiver.Tag().String(),
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
func (a *ActionAPI) ServicesCharmActions(args params.Entities) (params.ServicesCharmActionsResults, error) {
	result := params.ServicesCharmActionsResults{}
	for _, entity := range args.Entities {
		newResult := params.ServiceCharmActionsResult{}
		svcTag, err := names.ParseServiceTag(entity.Tag)
		if err != nil {
			newResult.Error = common.ServerError(common.ErrBadId)
			result.Results = append(result.Results, newResult)
			continue
		}
		newResult.ServiceTag = svcTag.String()
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

// internalList takes a list of Entities representing ActionReceivers and
// returns all of the Actions the extractorFn can get out of the
// ActionReceiver.
func (a *ActionAPI) internalList(arg params.Entities, fn extractorFn) (params.ActionsByReceivers, error) {
	response := params.ActionsByReceivers{Actions: make([]params.ActionsByReceiver, len(arg.Entities))}
	for i, entity := range arg.Entities {
		current := &response.Actions[i]
		receiver, err := tagToActionReceiver(a.state, entity.Tag)
		if err != nil {
			current.Error = common.ServerError(common.ErrBadId)
			continue
		}
		current.Receiver = receiver.Tag().String()

		results, err := fn(receiver)
		if err != nil {
			current.Error = common.ServerError(err)
			continue
		}
		current.Actions = results
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
	actions, err := ar.PendingActions()
	if err != nil {
		return items, err
	}
	for _, action := range actions {
		if action == nil {
			continue
		}
		items = append(items, params.ActionResult{
			Action: &params.Action{
				Receiver:   ar.Tag().String(),
				Tag:        action.ActionTag().String(),
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
	actions, err := ar.CompletedActions()
	if err != nil {
		return items, err
	}
	for _, action := range actions {
		if action == nil {
			continue
		}
		output, message := action.Results()
		items = append(items, params.ActionResult{
			Action: &params.Action{
				Receiver:   ar.Tag().String(),
				Tag:        action.ActionTag().String(),
				Name:       action.Name(),
				Parameters: action.Parameters(),
			},
			Status:  string(action.Status()),
			Message: message,
			Output:  output,
		})
	}
	return items, nil
}
