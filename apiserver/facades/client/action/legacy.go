// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// TODO(juju3) - no juju api clients use these methods, remove in juju v3.

// ListAll takes a list of Entities representing ActionReceivers and
// returns all of the Actions that have been enqueued or run by each of
// those Entities.
func (a *APIv4) ListAll(arg params.Entities) (params.ActionsByReceivers, error) {
	return a.listAll(arg, true)
}

// ListAll takes a list of Entities representing ActionReceivers and
// returns all of the Actions that have been enqueued or run by each of
// those Entities.
func (a *ActionAPI) ListAll(arg params.Entities) (params.ActionsByReceivers, error) {
	return a.listAll(arg, false)
}

func (a *ActionAPI) listAll(arg params.Entities, compat bool) (params.ActionsByReceivers, error) {
	if err := a.checkCanRead(); err != nil {
		return params.ActionsByReceivers{}, errors.Trace(err)
	}

	return a.internalList(arg, combine(pendingActions, runningActions, completedActions), compat)
}

// ListPending takes a list of Entities representing ActionReceivers
// and returns all of the Actions that are enqueued for each of those
// Entities.
func (a *ActionAPI) ListPending(arg params.Entities) (params.ActionsByReceivers, error) {
	if err := a.checkCanRead(); err != nil {
		return params.ActionsByReceivers{}, errors.Trace(err)
	}

	return a.internalList(arg, pendingActions, false)
}

// ListRunning takes a list of Entities representing ActionReceivers and
// returns all of the Actions that have are running on each of those
// Entities.
func (a *ActionAPI) ListRunning(arg params.Entities) (params.ActionsByReceivers, error) {
	if err := a.checkCanRead(); err != nil {
		return params.ActionsByReceivers{}, errors.Trace(err)
	}

	return a.internalList(arg, runningActions, false)
}

// ListCompleted takes a list of Entities representing ActionReceivers
// and returns all of the Actions that have been run on each of those
// Entities.
func (a *APIv4) ListCompleted(arg params.Entities) (params.ActionsByReceivers, error) {
	return a.listCompleted(arg, true)
}

// ListCompleted takes a list of Entities representing ActionReceivers
// and returns all of the Actions that have been run on each of those
// Entities.
func (a *ActionAPI) ListCompleted(arg params.Entities) (params.ActionsByReceivers, error) {
	return a.listCompleted(arg, false)
}

func (a *ActionAPI) listCompleted(arg params.Entities, compat bool) (params.ActionsByReceivers, error) {
	if err := a.checkCanRead(); err != nil {
		return params.ActionsByReceivers{}, errors.Trace(err)
	}

	return a.internalList(arg, completedActions, compat)
}

// internalList takes a list of Entities representing ActionReceivers
// and returns all of the Actions the extractorFn can get out of the
// ActionReceiver.
func (a *ActionAPI) internalList(arg params.Entities, fn extractorFn, compat bool) (params.ActionsByReceivers, error) {
	tagToActionReceiver := common.TagToActionReceiverFn(a.state.FindEntity)
	response := params.ActionsByReceivers{Actions: make([]params.ActionsByReceiver, len(arg.Entities))}
	for i, entity := range arg.Entities {
		currentResult := &response.Actions[i]
		receiver, err := tagToActionReceiver(entity.Tag)
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}
		currentResult.Receiver = receiver.Tag().String()

		results, err := fn(receiver, compat)
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(err)
			continue
		}
		currentResult.Actions = results
	}
	return response, nil
}

// extractorFn is the generic signature for functions that extract
// state.Actions from an ActionReceiver, and return them as a slice of
// params.ActionResult.
type extractorFn func(state.ActionReceiver, bool) ([]params.ActionResult, error)

// combine takes multiple extractorFn's and combines them into one
// function.
func combine(funcs ...extractorFn) extractorFn {
	return func(ar state.ActionReceiver, compat bool) ([]params.ActionResult, error) {
		result := []params.ActionResult{}
		for _, fn := range funcs {
			items, err := fn(ar, compat)
			if err != nil {
				return result, errors.Trace(err)
			}
			result = append(result, items...)
		}
		return result, nil
	}
}

// pendingActions iterates through the Actions() enqueued for an
// ActionReceiver, and converts them to a slice of params.ActionResult.
func pendingActions(ar state.ActionReceiver, compat bool) ([]params.ActionResult, error) {
	return common.ConvertActions(ar, ar.PendingActions, compat)
}

// runningActions iterates through the Actions() running on an
// ActionReceiver, and converts them to a slice of params.ActionResult.
func runningActions(ar state.ActionReceiver, compat bool) ([]params.ActionResult, error) {
	return common.ConvertActions(ar, ar.RunningActions, compat)
}

// completedActions iterates through the Actions() that have run to
// completion for an ActionReceiver, and converts them to a slice of
// params.ActionResult.
func completedActions(ar state.ActionReceiver, compat bool) ([]params.ActionResult, error) {
	return common.ConvertActions(ar, ar.CompletedActions, compat)
}
