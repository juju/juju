// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

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
// the designated Unit.
func (a *ActionsAPI) Enqueue(arg params.Actions) (params.ErrorResults, error) {
	return params.ErrorResults{}, errors.NotImplementedf("Enqueue")
}

// ListAll takes a list of Tags representing ActionReceivers and returns
// all of the Actions that have been queued or run by each of those
// Entities.
func (a *ActionsAPI) ListAll(arg params.Tags) (params.ActionsByTag, error) {
	response := params.ActionsByTag{Actions: make([]params.Actions, len(arg.Tags))}
	// TODO(jcw4) authorization checks
	for i, tag := range arg.Tags {
		cur := &response.Actions[i]
		entity, err := a.state.FindEntity(tag)
		if err != nil {
			cur.Error = common.ServerError(common.ErrBadId)
			continue
		}
		r, ok := entity.(state.ActionReceiver)
		if !ok {
			cur.Error = common.ServerError(common.ErrBadId)
			continue
		}
		cur.Receiver = entity.Tag()
		actions, err := r.Actions()
		if err != nil {
			cur.Error = common.ServerError(err)
			continue
		}
		cur.ActionItems = make([]params.ActionItem, len(actions))
		for j, action := range actions {
			item := &cur.ActionItems[j]
			actionTag := action.ActionTag()
			item.Tag = actionTag
			item.Name = action.Name()
			item.Parameters = action.Parameters()

			ar, err := a.state.ActionResultByTag(actionTag)
			if err != nil {
				item.Status = "pending"
				continue
			}
			output, message := ar.Results()
			item.Status = string(ar.Status())
			item.Message = message
			item.Output = output
		}

	}
	return response, nil
}

// ListPending takes a list of Tags representing ActionReceivers
// and returns all of the Actions that are queued for each of those
// Entities.
func (a *ActionsAPI) ListPending(arg params.Tags) (params.Actions, error) {
	return params.Actions{}, errors.NotImplementedf("ListPending")
}

// ListCompleted takes a list of Tags representing ActionReceivers
// and returns all of the Actions that have been run on each of those
// Entities.
func (a *ActionsAPI) ListCompleted(arg params.Tags) (params.Actions, error) {
	return params.Actions{}, errors.NotImplementedf("ListCompleted")
}

// Cancel attempts to cancel a queued up Action from running.
func (a *ActionsAPI) Cancel(arg params.ActionsRequest) (params.ErrorResults, error) {
	return params.ErrorResults{}, errors.NotImplementedf("Cancel")
}
