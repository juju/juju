// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/rpc/params"
)

// ApplicationService is used to interact with the application service.
type ApplicationService interface {
	// GetApplicationScale returns the desired scale of an application,
	GetApplicationScale(ctx context.Context, appName string) (int, error)

	// SetApplicationScale sets the application's desired scale value,
	SetApplicationScale(ctx context.Context, appName string, scale int) error

	// UpsertCloudService updates the cloud service for the specified application.
	UpdateCloudService(ctx context.Context, appName, providerID string, sAddrs network.ProviderAddresses) error

	// WatchApplicationScale returns a watcher that observes changes to an application's scale.
	WatchApplicationScale(ctx context.Context, appName string) (watcher.NotifyWatcher, error)
}

type Facade struct {
	watcherRegistry facade.WatcherRegistry

	applicationService ApplicationService
	resources          facade.Resources
	clock              clock.Clock
	logger             corelogger.Logger
}

// NewFacade returns a new CAAS unit provisioner Facade facade.
func NewFacade(
	watcherRegistry facade.WatcherRegistry,
	resources facade.Resources,
	authorizer facade.Authorizer,
	applicationService ApplicationService,
	clock clock.Clock,
	logger corelogger.Logger,
) (*Facade, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &Facade{
		watcherRegistry:    watcherRegistry,
		applicationService: applicationService,
		resources:          resources,
		clock:              clock,
		logger:             logger,
	}, nil
}

// WatchApplicationsScale starts a NotifyWatcher to watch changes
// to the applications' scale.
func (f *Facade) WatchApplicationsScale(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		id, err := f.watchApplicationScale(ctx, arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].NotifyWatcherId = id
	}
	return results, nil
}

func (f *Facade) watchApplicationScale(ctx context.Context, tagString string) (string, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return "", errors.Trace(err)
	}
	w, err := f.applicationService.WatchApplicationScale(ctx, tag.Id())
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return "", errors.NotFoundf("application %s not found", tag.Id())
	} else if err != nil {
		return "", errors.Trace(err)
	}
	notifyWatcherId, _, err := internal.EnsureRegisterWatcher(ctx, f.watcherRegistry, w)
	return notifyWatcherId, errors.Trace(err)
}

func (f *Facade) ApplicationsScale(ctx context.Context, args params.Entities) (params.IntResults, error) {
	results := params.IntResults{
		Results: make([]params.IntResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		scale, err := f.applicationScale(ctx, arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = scale
	}
	f.logger.Debugf(ctx, "application scale result: %#v", results)
	return results, nil
}

func (f *Facade) applicationScale(ctx context.Context, tagString string) (int, error) {
	appTag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return 0, errors.Trace(err)
	}
	scale, err := f.applicationService.GetApplicationScale(ctx, appTag.Id())
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return 0, errors.NotFoundf("application %s", appTag.Id())
	} else if err != nil {
		return 0, errors.Trace(err)
	}
	return scale, nil
}

// UpdateApplicationsService updates the Juju data model to reflect the given
// service details of the specified application.
func (f *Facade) UpdateApplicationsService(ctx context.Context, args params.UpdateApplicationServiceArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	if len(args.Args) == 0 {
		return result, nil
	}
	for i, appUpdate := range args.Args {
		appTag, err := names.ParseApplicationTag(appUpdate.ApplicationTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		pas := params.ToProviderAddresses(appUpdate.Addresses...)

		appName := appTag.Id()
		err = f.applicationService.UpdateCloudService(ctx, appName, appUpdate.ProviderId, pas)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			result.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "application %s not found", appName)
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
		if appUpdate.Scale != nil {
			err = f.applicationService.SetApplicationScale(ctx, appName, *appUpdate.Scale)
			if errors.Is(err, applicationerrors.ApplicationNotFound) {
				result.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "application %s not found", appName)
			} else if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
			}
		}
	}
	return result, nil
}
