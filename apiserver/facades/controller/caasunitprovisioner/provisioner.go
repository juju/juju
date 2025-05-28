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
	coreapplication "github.com/juju/juju/core/application"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/rpc/params"
	statewatcher "github.com/juju/juju/state/watcher"
)

// ApplicationService is used to interact with the application service.
type ApplicationService interface {
	GetApplicationScale(ctx context.Context, appName string) (int, error)
	SetApplicationScale(ctx context.Context, appName string, scale int) error
	UpdateCloudService(ctx context.Context, appName, providerID string, sAddrs network.ProviderAddresses) error
	WatchApplicationScale(ctx context.Context, appName string) (watcher.NotifyWatcher, error)
}

type Facade struct {
	watcherRegistry facade.WatcherRegistry

	applicationService ApplicationService
	resources          facade.Resources
	state              CAASUnitProvisionerState
	clock              clock.Clock
	logger             corelogger.Logger
}

// NewFacade returns a new CAAS unit provisioner Facade facade.
func NewFacade(
	watcherRegistry facade.WatcherRegistry,
	resources facade.Resources,
	authorizer facade.Authorizer,
	applicationService ApplicationService,
	st CAASUnitProvisionerState,
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
		state:              st,
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
	if err != nil {
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
	if err != nil {
		return 0, errors.Trace(err)
	}
	return scale, nil
}

// ApplicationsTrust returns the trust status for specified applications in this model.
func (f *Facade) ApplicationsTrust(ctx context.Context, args params.Entities) (params.BoolResults, error) {
	results := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		trust, err := f.applicationTrust(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = trust
	}
	f.logger.Debugf(ctx, "application trust result: %#v", results)
	return results, nil
}

func (f *Facade) applicationTrust(tagString string) (bool, error) {
	appTag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return false, errors.Trace(err)
	}
	app, err := f.state.Application(appTag.Id())
	if err != nil {
		return false, errors.Trace(err)
	}
	cfg, err := app.ApplicationConfig()
	if err != nil {
		return false, errors.Trace(err)
	}
	return cfg.GetBool(coreapplication.TrustConfigOptionName, false), nil
}

// WatchApplicationsTrustHash starts a StringsWatcher to watch changes
// to the applications' trust status.
func (f *Facade) WatchApplicationsTrustHash(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		id, err := f.watchApplicationTrustHash(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].StringsWatcherId = id
	}
	return results, nil
}

func (f *Facade) watchApplicationTrustHash(tagString string) (string, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return "", errors.Trace(err)
	}
	app, err := f.state.Application(tag.Id())
	if err != nil {
		return "", errors.Trace(err)
	}
	// This is currently implemented by just watching the
	// app config settings which is where the trust value
	// is stored. A similar pattern is used for model config
	// watchers pending better filtering on watchers.
	w := app.WatchConfigSettingsHash()
	if _, ok := <-w.Changes(); ok {
		return f.resources.Register(w), nil
	}
	return "", statewatcher.EnsureErr(w)
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
		if err := f.applicationService.UpdateCloudService(ctx, appName, appUpdate.ProviderId, pas); err != nil {
			if errors.Is(err, applicationerrors.ApplicationNotFound) {
				err = errors.NotFoundf("application %s not found", appName)
			}
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
		if appUpdate.Scale != nil {
			if err := f.applicationService.SetApplicationScale(ctx, appName, *appUpdate.Scale); err != nil {
				if errors.Is(err, applicationerrors.ApplicationNotFound) {
					err = errors.NotFoundf("application %s not found", appName)
				}
				result.Results[i].Error = apiservererrors.ServerError(err)
			}
		}
	}
	return result, nil
}
