// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// ApplicationService is an interface that provides access to application
// entities.
type ApplicationService interface {
	// GetCharmLocatorByApplicationName returns a CharmLocator by application name.
	// It returns an error if the charm can not be found by the name. This can also
	// be used as a cheap way to see if a charm exists without needing to load the
	// charm metadata.
	GetCharmLocatorByApplicationName(ctx context.Context, name string) (applicationcharm.CharmLocator, error)

	// GetCharmActions returns the actions for the charm using the charm name,
	// source and revision.
	//
	// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmActions(ctx context.Context, locator applicationcharm.CharmLocator) (internalcharm.Actions, error)
}

// ModelInfoService provides access to information about the model.
type ModelInfoService interface {
	// GetModelInfo returns information about the current model.
	GetModelInfo(context.Context) (coremodel.ModelInfo, error)
}

// ActionAPI implements the client API for interacting with Actions
type ActionAPI struct {
	state              State
	model              Model
	modelTag           names.ModelTag
	modelInfoService   ModelInfoService
	resources          facade.Resources
	authorizer         facade.Authorizer
	check              *common.BlockChecker
	leadership         leadership.Reader
	applicationService ApplicationService

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
	getLeadershipReader func() (leadership.Reader, error),
	applicationService ApplicationService,
	blockCommandService common.BlockCommandService,
	modelInfoService ModelInfoService,
	modelUUID coremodel.UUID,
) (*ActionAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	leaders, err := getLeadershipReader()
	if err != nil {
		return nil, errors.Trace(err)
	}

	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelTag := names.NewModelTag(modelUUID.String())

	return &ActionAPI{
		state:                 st,
		model:                 m,
		modelTag:              modelTag,
		modelInfoService:      modelInfoService,
		resources:             resources,
		authorizer:            authorizer,
		check:                 common.NewBlockChecker(blockCommandService),
		leadership:            leaders,
		tagToActionReceiverFn: common.TagToActionReceiverFn,
		applicationService:    applicationService,
	}, nil
}

func (a *ActionAPI) checkCanRead(ctx context.Context) error {
	return a.authorizer.HasPermission(ctx, permission.ReadAccess, a.modelTag)
}

func (a *ActionAPI) checkCanWrite(ctx context.Context) error {
	return a.authorizer.HasPermission(ctx, permission.WriteAccess, a.modelTag)
}

func (a *ActionAPI) checkCanAdmin(ctx context.Context) error {
	return a.authorizer.HasPermission(ctx, permission.AdminAccess, a.modelTag)
}

// Actions takes a list of ActionTags, and returns the full Action for
// each ID.
func (a *ActionAPI) Actions(ctx context.Context, arg params.Entities) (params.ActionResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
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
		action, err := a.state.ActionByTag(actionTag)
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
	if err := a.checkCanWrite(ctx); err != nil {
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

		action, err := a.state.ActionByTag(actionTag)
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
	if err := a.checkCanWrite(ctx); err != nil {
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

		locator, err := a.applicationService.GetCharmLocatorByApplicationName(ctx, svcTag.Id())
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			currentResult.Error = apiservererrors.ServerError(errors.NotFoundf("application %q", svcTag.Id()))
			continue
		} else if err != nil {
			currentResult.Error = apiservererrors.ServerError(err)
			continue
		}

		actions, err := a.applicationService.GetCharmActions(ctx, locator)
		if errors.Is(err, applicationerrors.CharmNotFound) {
			currentResult.Error = apiservererrors.ServerError(errors.NotFoundf("charm %q", locator))
			continue
		} else if err != nil {
			currentResult.Error = apiservererrors.ServerError(err)
			continue
		}

		charmActions := make(map[string]params.ActionSpec)
		for key, value := range actions.ActionSpecs {
			charmActions[key] = params.ActionSpec{
				Description: value.Description,
				Params:      value.Params,
			}
		}
		currentResult.Actions = charmActions
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
