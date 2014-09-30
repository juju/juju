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

// Enqueue takes a list of Actions and queues them up to be executed by the designated Unit.
func (a *ActionsAPI) Enqueue(arg params.Actions) (params.ErrorResults, error) {
	return params.ErrorResults{}, errors.Errorf("not implemented")
}

// ListAll takes a list of Entities and returns all of the Actions that have been queued
// or run by that Entity.
func (a *ActionsAPI) ListAll(arg params.Entities) (params.Actions, error) {
	return params.Actions{}, errors.Errorf("not implemented")
}

// ListPending takes a list of Entities and returns all of the Actions that are queued for
// that Entity.
func (a *ActionsAPI) ListPending(arg params.Entities) (params.Actions, error) {
	return params.Actions{}, errors.Errorf("not implemented")
}

// ListCompleted takes a list of Entities and returns all of the Actions that have been
// run on that Entity.
func (a *ActionsAPI) ListCompleted(arg params.Entities) (params.Actions, error) {
	return params.Actions{}, errors.Errorf("not implemented")
}

// Cancel attempts to cancel a queued up Action from running.
func (a *ActionsAPI) Cancel(arg params.ActionsRequest) (params.ErrorResults, error) {
	return params.ErrorResults{}, errors.Errorf("not implemented")
}
