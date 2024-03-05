// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/watcher"
)

type Facade struct {
	resources facade.Resources
	state     CAASUnitProvisionerState
	clock     clock.Clock
	logger    loggo.Logger
}

// NewFacade returns a new CAAS unit provisioner Facade facade.
func NewFacade(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st CAASUnitProvisionerState,
	clock clock.Clock,
	logger loggo.Logger,
) (*Facade, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &Facade{
		resources: resources,
		state:     st,
		clock:     clock,
		logger:    logger,
	}, nil
}

// WatchApplicationsScale starts a NotifyWatcher to watch changes
// to the applications' scale.
func (f *Facade) WatchApplicationsScale(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		id, err := f.watchApplicationScale(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].NotifyWatcherId = id
	}
	return results, nil
}

func (f *Facade) watchApplicationScale(tagString string) (string, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return "", errors.Trace(err)
	}
	app, err := f.state.Application(tag.Id())
	if err != nil {
		return "", errors.Trace(err)
	}
	w := app.WatchScale()
	if _, ok := <-w.Changes(); ok {
		return f.resources.Register(w), nil
	}
	return "", watcher.EnsureErr(w)
}

func (f *Facade) ApplicationsScale(ctx context.Context, args params.Entities) (params.IntResults, error) {
	results := params.IntResults{
		Results: make([]params.IntResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		scale, err := f.applicationScale(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = scale
	}
	f.logger.Debugf("application scale result: %#v", results)
	return results, nil
}

func (f *Facade) applicationScale(tagString string) (int, error) {
	appTag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return 0, errors.Trace(err)
	}
	app, err := f.state.Application(appTag.Id())
	if err != nil {
		return 0, errors.Trace(err)
	}
	return app.GetScale(), nil
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
	f.logger.Debugf("application trust result: %#v", results)
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
	return "", watcher.EnsureErr(w)
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
		app, err := f.state.Application(appTag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		sAddrs, err := params.ToProviderAddresses(appUpdate.Addresses...).ToSpaceAddresses(f.state)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if err := app.UpdateCloudService(appUpdate.ProviderId, sAddrs); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
		if appUpdate.Scale != nil {
			var generation int64
			if appUpdate.Generation != nil {
				generation = *appUpdate.Generation
			}
			if err := app.SetScale(*appUpdate.Scale, generation, false); err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
			}
		}
	}
	return result, nil
}
