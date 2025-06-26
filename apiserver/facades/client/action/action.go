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
	coreunit "github.com/juju/juju/core/unit"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
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

	// GetAllUnitNames returns a slice of all unit names in the model.
	GetAllUnitNames(ctx context.Context) ([]coreunit.Name, error)

	// GetUnitNamesForApplication returns a slice of the unit names for the given application
	// The following errors may be returned:
	// - [applicationerrors.ApplicationIsDead] if the application is dead
	// - [applicationerrors.ApplicationNotFound] if the application does not exist
	GetUnitNamesForApplication(ctx context.Context, name string) ([]coreunit.Name, error)
}

// ModelInfoService provides access to information about the model.
type ModelInfoService interface {
	// GetModelInfo returns information about the current model.
	GetModelInfo(context.Context) (coremodel.ModelInfo, error)
}

// ActionAPI implements the client API for interacting with Actions
type ActionAPI struct {
	modelTag           names.ModelTag
	modelInfoService   ModelInfoService
	resources          facade.Resources
	authorizer         facade.Authorizer
	check              *common.BlockChecker
	leadership         leadership.Reader
	applicationService ApplicationService
}

// APIv7 provides the Action API facade for version 7.
type APIv7 struct {
	*ActionAPI
}

func newActionAPI(
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

	modelTag := names.NewModelTag(modelUUID.String())

	return &ActionAPI{
		modelTag:           modelTag,
		modelInfoService:   modelInfoService,
		resources:          resources,
		authorizer:         authorizer,
		check:              common.NewBlockChecker(blockCommandService),
		leadership:         leaders,
		applicationService: applicationService,
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
		_, ok := tag.(names.ActionTag)
		if !ok {
			currentResult.Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}

		currentResult.Error = apiservererrors.ServerError(errors.NotSupportedf("actions in Dqlite"))
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
		_, ok := tag.(names.ActionTag)
		if !ok {
			currentResult.Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}

		currentResult.Error = apiservererrors.ServerError(errors.NotSupportedf("actions in Dqlite"))
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
		_, err := names.ParseActionTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		results.Results[i].Error = apiservererrors.ServerError(errors.NotSupportedf("actions in Dqlite"))
	}
	return results, nil
}
