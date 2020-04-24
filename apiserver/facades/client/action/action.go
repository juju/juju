// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// ActionAPI implements the client API for interacting with Actions
type ActionAPI struct {
	state      *state.State
	model      *state.Model
	resources  facade.Resources
	authorizer facade.Authorizer
	check      *common.BlockChecker
}

// APIv2 provides the Action API facade for version 2.
type APIv2 struct {
	*APIv3
}

// APIv3 provides the Action API facade for version 3.
type APIv3 struct {
	*APIv4
}

// APIv4 provides the Action API facade for version 4.
type APIv4 struct {
	*APIv5
}

// APIv5 provides the Action API facade for version 5.
type APIv5 struct {
	*APIv6
}

// APIv6 provides the Action API facade for version 6.
type APIv6 struct {
	*ActionAPI
}

// NewActionAPIV2 returns an initialized ActionAPI for version 2.
func NewActionAPIV2(ctx facade.Context) (*APIv2, error) {
	api, err := NewActionAPIV3(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}

// NewActionAPIV3 returns an initialized ActionAPI for version 3.
func NewActionAPIV3(ctx facade.Context) (*APIv3, error) {
	api, err := NewActionAPIV4(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv3{api}, nil
}

// NewActionAPIV4 returns an initialized ActionAPI for version 4.
func NewActionAPIV4(ctx facade.Context) (*APIv4, error) {
	api, err := NewActionAPIV5(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv4{api}, nil
}

// NewActionAPIV5 returns an initialized ActionAPI for version 5.
func NewActionAPIV5(ctx facade.Context) (*APIv5, error) {
	api, err := NewActionAPIV6(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv5{api}, nil
}

// NewActionAPIV6 returns an initialized ActionAPI for version 6.
func NewActionAPIV6(ctx facade.Context) (*APIv6, error) {
	api, err := newActionAPI(ctx.State(), ctx.Resources(), ctx.Auth())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv6{api}, nil
}

func newActionAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*ActionAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &ActionAPI{
		state:      st,
		model:      m,
		resources:  resources,
		authorizer: authorizer,
		check:      common.NewBlockChecker(st),
	}, nil
}

func (a *ActionAPI) checkCanRead() error {
	canRead, err := a.authorizer.HasPermission(permission.ReadAccess, a.model.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canRead {
		return common.ErrPerm
	}
	return nil
}

func (a *ActionAPI) checkCanWrite() error {
	canWrite, err := a.authorizer.HasPermission(permission.WriteAccess, a.model.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canWrite {
		return common.ErrPerm
	}
	return nil
}

func (a *ActionAPI) checkCanAdmin() error {
	canAdmin, err := a.authorizer.HasPermission(permission.AdminAccess, a.model.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canAdmin {
		return common.ErrPerm
	}
	return nil
}

// Actions takes a list of ActionTags, and returns the full Action for
// each ID.
func (a *APIv4) Actions(arg params.Entities) (params.ActionResults, error) {
	return a.actions(arg, true)
}

// Actions takes a list of ActionTags, and returns the full Action for
// each ID.
func (a *ActionAPI) Actions(arg params.Entities) (params.ActionResults, error) {
	return a.actions(arg, false)
}

// Actions takes a list of ActionTags, and returns the full Action for
// each ID.
func (a *ActionAPI) actions(arg params.Entities, compat bool) (params.ActionResults, error) {
	if err := a.checkCanRead(); err != nil {
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
		m, err := a.state.Model()
		if err != nil {
			return params.ActionResults{}, errors.Trace(err)
		}
		action, err := m.ActionByTag(actionTag)
		if err != nil {
			currentResult.Error = common.ServerError(common.ErrBadId)
			continue
		}
		receiverTag, err := names.ActionReceiverTag(action.Receiver())
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		response.Results[i] = common.MakeActionResult(receiverTag, action, compat)
	}
	return response, nil
}

// FindActionTagsByPrefix takes a list of string prefixes and finds
// corresponding ActionTags that match that prefix.
// TODO(juju3) - rename API method since we only need prefix matching for UUIDs
func (a *ActionAPI) FindActionTagsByPrefix(arg params.FindTags) (params.FindTagsResults, error) {
	if err := a.checkCanRead(); err != nil {
		return params.FindTagsResults{}, errors.Trace(err)
	}

	response := params.FindTagsResults{Matches: make(map[string][]params.Entity)}
	for _, prefix := range arg.Prefixes {
		m, err := a.state.Model()
		if err != nil {
			return params.FindTagsResults{}, errors.Trace(err)
		}
		found, err := m.FindActionTagsById(prefix)
		if err != nil {
			return params.FindTagsResults{}, errors.Trace(err)
		}
		matches := make([]params.Entity, len(found))
		for i, tag := range found {
			matches[i] = params.Entity{Tag: tag.String()}
		}
		response.Matches[prefix] = matches
	}
	return response, nil
}

func (a *ActionAPI) FindActionsByNames(arg params.FindActionsByNames) (params.ActionsByNames, error) {
	if err := a.checkCanWrite(); err != nil {
		return params.ActionsByNames{}, errors.Trace(err)
	}

	response := params.ActionsByNames{Actions: make([]params.ActionsByName, len(arg.ActionNames))}
	for i, name := range arg.ActionNames {
		currentResult := &response.Actions[i]
		currentResult.Name = name

		m, err := a.state.Model()
		if err != nil {
			return params.ActionsByNames{}, errors.Trace(err)
		}

		actions, err := m.FindActionsByName(name)
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
			currentAction := common.MakeActionResult(recvTag, action, true)
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
	_, results, err := a.enqueue(arg)
	return results, err
}

// Cancel attempts to cancel enqueued Actions from running.
func (a *APIv4) Cancel(arg params.Entities) (params.ActionResults, error) {
	return a.cancel(arg, true)
}

// Cancel attempts to cancel enqueued Actions from running.
func (a *ActionAPI) Cancel(arg params.Entities) (params.ActionResults, error) {
	return a.cancel(arg, false)
}

func (a *ActionAPI) cancel(arg params.Entities, compat bool) (params.ActionResults, error) {
	if err := a.checkCanWrite(); err != nil {
		return params.ActionResults{}, errors.Trace(err)
	}

	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Entities))}

	for i, entity := range arg.Entities {
		currentResult := &response.Results[i]
		currentResult.Action = &params.Action{Tag: entity.Tag}
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

		m, err := a.state.Model()
		if err != nil {
			return params.ActionResults{}, errors.Trace(err)
		}

		action, err := m.ActionByTag(actionTag)
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		result, err := action.Cancel()
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		receiverTag, err := names.ActionReceiverTag(result.Receiver())
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}

		response.Results[i] = common.MakeActionResult(receiverTag, result, compat)
	}
	return response, nil
}

// ApplicationsCharmsActions returns a slice of charm Actions for a slice of
// services.
func (a *ActionAPI) ApplicationsCharmsActions(args params.Entities) (params.ApplicationsCharmActionsResults, error) {
	result := params.ApplicationsCharmActionsResults{Results: make([]params.ApplicationCharmActionsResult, len(args.Entities))}
	if err := a.checkCanWrite(); err != nil {
		return result, errors.Trace(err)
	}

	for i, entity := range args.Entities {
		currentResult := &result.Results[i]
		svcTag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			currentResult.Error = common.ServerError(common.ErrBadId)
			continue
		}
		currentResult.ApplicationTag = svcTag.String()
		svc, err := a.state.Application(svcTag.Id())
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		ch, _, err := svc.Charm()
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		if actions := ch.Actions(); actions != nil {
			charmActions := make(map[string]params.ActionSpec)
			for key, value := range actions.ActionSpecs {
				charmActions[key] = params.ActionSpec{
					Description: value.Description,
					Params:      value.Params,
				}
			}
			currentResult.Actions = charmActions
		}
	}
	return result, nil
}

// WatchActionsProgress creates a watcher that reports on action log messages.
func (api *ActionAPI) WatchActionsProgress(actions params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(actions.Entities)),
	}
	for i, arg := range actions.Entities {
		actionTag, err := names.ParseActionTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}

		w := api.state.WatchActionLogs(actionTag.Id())
		// Consume the initial event.
		changes, ok := <-w.Changes()
		if !ok {
			results.Results[i].Error = common.ServerError(watcher.EnsureErr(w))
			continue
		}

		results.Results[i].Changes = changes
		results.Results[i].StringsWatcherId = api.resources.Register(w)
	}
	return results, nil
}
