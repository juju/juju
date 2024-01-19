// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	coreuser "github.com/juju/juju/core/user"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// ActionAPI implements the client API for interacting with Actions
type ActionAPI struct {
	state      State
	model      Model
	resources  facade.Resources
	authorizer facade.Authorizer
	check      *common.BlockChecker
	leadership leadership.Reader

	tagToActionReceiverFn TagToActionReceiverFunc
}

type TagToActionReceiverFunc func(findEntity func(names.Tag) (state.Entity, error)) func(tag string) (state.ActionReceiver, error)

// APIv7 provides the Action API facade for version 7.
type APIv7 struct {
	*ActionAPI
}

func newActionAPI(
	st State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	getLeadershipReader func(string) (leadership.Reader, error),
) (*ActionAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	leaders, err := getLeadershipReader(m.ModelTag().Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &ActionAPI{
		state:                 st,
		model:                 m,
		resources:             resources,
		authorizer:            authorizer,
		check:                 common.NewBlockChecker(st),
		leadership:            leaders,
		tagToActionReceiverFn: common.TagToActionReceiverFn,
	}, nil
}

func (a *ActionAPI) checkCanRead(usr coreuser.User) error {
	return a.authorizer.HasPermission(usr, permission.ReadAccess, a.model.ModelTag())
}

func (a *ActionAPI) checkCanWrite(usr coreuser.User) error {
	return a.authorizer.HasPermission(usr, permission.WriteAccess, a.model.ModelTag())
}

func (a *ActionAPI) checkCanAdmin(usr coreuser.User) error {
	return a.authorizer.HasPermission(usr, permission.AdminAccess, a.model.ModelTag())
}

// Actions takes a list of ActionTags, and returns the full Action for
// each ID.
func (a *ActionAPI) Actions(ctx context.Context, arg params.Entities) (params.ActionResults, error) {
	if err := a.checkCanRead(usr); err != nil {
		return params.ActionResults{}, errors.Trace(err)
	}

	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Entities))}
	for i, entity := range arg.Entities {
		currentResult := &response.Results[i]
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}
		actionTag, ok := tag.(names.ActionTag)
		if !ok {
			currentResult.Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}
		m, err := a.state.Model()
		if err != nil {
			return params.ActionResults{}, errors.Trace(err)
		}
		action, err := m.ActionByTag(actionTag)
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}
		receiverTag, err := names.ActionReceiverTag(action.Receiver())
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(err)
			continue
		}
		response.Results[i] = common.MakeActionResult(receiverTag, action)
	}
	return response, nil
}

// Cancel attempts to cancel enqueued Actions from running.
func (a *ActionAPI) Cancel(ctx context.Context, arg params.Entities) (params.ActionResults, error) {
	if err := a.checkCanWrite(usr); err != nil {
		return params.ActionResults{}, errors.Trace(err)
	}

	response := params.ActionResults{Results: make([]params.ActionResult, len(arg.Entities))}

	for i, entity := range arg.Entities {
		currentResult := &response.Results[i]
		currentResult.Action = &params.Action{Tag: entity.Tag}
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}
		actionTag, ok := tag.(names.ActionTag)
		if !ok {
			currentResult.Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}

		m, err := a.state.Model()
		if err != nil {
			return params.ActionResults{}, errors.Trace(err)
		}

		action, err := m.ActionByTag(actionTag)
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(err)
			continue
		}
		result, err := action.Cancel()
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(err)
			continue
		}
		receiverTag, err := names.ActionReceiverTag(result.Receiver())
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(err)
			continue
		}

		response.Results[i] = common.MakeActionResult(receiverTag, result)
	}
	return response, nil
}

// ApplicationsCharmsActions returns a slice of charm Actions for a slice of
// services.
func (a *ActionAPI) ApplicationsCharmsActions(ctx context.Context, args params.Entities) (params.ApplicationsCharmActionsResults, error) {
	result := params.ApplicationsCharmActionsResults{Results: make([]params.ApplicationCharmActionsResult, len(args.Entities))}
	if err := a.checkCanWrite(usr); err != nil {
		return result, errors.Trace(err)
	}

	for i, entity := range args.Entities {
		currentResult := &result.Results[i]
		svcTag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}
		currentResult.ApplicationTag = svcTag.String()
		svc, err := a.state.Application(svcTag.Id())
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(err)
			continue
		}
		ch, _, err := svc.Charm()
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(err)
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
func (api *ActionAPI) WatchActionsProgress(ctx context.Context, actions params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(actions.Entities)),
	}
	for i, arg := range actions.Entities {
		actionTag, err := names.ParseActionTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		w := api.state.WatchActionLogs(actionTag.Id())
		// Consume the initial event.
		changes, ok := <-w.Changes()
		if !ok {
			results.Results[i].Error = apiservererrors.ServerError(watcher.EnsureErr(w))
			continue
		}

		results.Results[i].Changes = changes
		results.Results[i].StringsWatcherId = api.resources.Register(w)
	}
	return results, nil
}
