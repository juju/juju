// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/proxy"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/rpc/params"
)

// ProxyUpdaterV2 defines the public methods for the v2 facade.
type ProxyUpdaterV2 interface {
	ProxyConfig(ctx context.Context, args params.Entities) params.ProxyConfigResults
	WatchForProxyConfigAndAPIHostPortChanges(ctx context.Context, args params.Entities) params.NotifyWatchResults
}

var _ ProxyUpdaterV2 = (*API)(nil)

// newFacadeBase provides the signature required for facade registration
// and creates a v2 facade.
func newFacadeBase(ctx facade.ModelContext) (*API, error) {
	return NewAPIV2(
		ctx.DomainServices().ControllerNode(),
		ctx.DomainServices().Config(),
		ctx.Auth(),
		ctx.WatcherRegistry(),
	)
}

// API provides the ProxyUpdater version 2 facade.
type API struct {
	controllerNodeService ControllerNodeService
	modelConfigService    ModelConfigService
	authorizer            facade.Authorizer
	watcherRegistry       facade.WatcherRegistry
}

// NewAPIV2 creates a new server-side API facade with the given Backing.
func NewAPIV2(
	controllerNodeService ControllerNodeService,
	modelConfigService ModelConfigService,
	authorizer facade.Authorizer,
	watcherRegistry facade.WatcherRegistry,
) (*API, error) {
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent() || authorizer.AuthApplicationAgent() || authorizer.AuthModelAgent()) {
		return nil, apiservererrors.ErrPerm
	}
	return &API{
		controllerNodeService: controllerNodeService,
		modelConfigService:    modelConfigService,
		authorizer:            authorizer,
		watcherRegistry:       watcherRegistry,
	}, nil
}

func (api *API) oneWatch(ctx context.Context) params.NotifyWatchResult {
	var result params.NotifyWatchResult

	modelConfigWatcher, err := api.modelConfigService.Watch()
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	modelConfigNotifyWatcher, err := watcher.Normalise(modelConfigWatcher)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	controllerAPIHostPortsWatcher, err := api.controllerNodeService.WatchControllerAPIAddresses(ctx)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	watch, err := eventsource.NewMultiNotifyWatcher(ctx,
		modelConfigNotifyWatcher,
		controllerAPIHostPortsWatcher,
	)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	result.NotifyWatcherId, _, err = internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, watch)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	return result
}

// WatchForProxyConfigAndAPIHostPortChanges watches for changes to the proxy and api host port settings.
func (api *API) WatchForProxyConfigAndAPIHostPortChanges(ctx context.Context, args params.Entities) params.NotifyWatchResults {
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	errors, _ := api.authEntities(args)

	for i := range args.Entities {
		if errors.Results[i].Error == nil {
			results.Results[i] = api.oneWatch(ctx)
		} else {
			results.Results[i].Error = errors.Results[i].Error
		}
	}

	return results
}

func toParams(settings proxy.Settings) params.ProxyConfig {
	return params.ProxyConfig{
		HTTP:    settings.Http,
		HTTPS:   settings.Https,
		FTP:     settings.Ftp,
		NoProxy: settings.FullNoProxy(),
	}
}

func (api *API) authEntities(args params.Entities) (params.ErrorResults, bool) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}

	var ok bool

	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if !api.authorizer.AuthOwner(tag) {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		ok = true
	}
	return result, ok
}

func (api *API) proxyConfig(ctx context.Context) params.ProxyConfigResult {
	var result params.ProxyConfigResult
	config, err := api.modelConfigService.ModelConfig(ctx)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	proxyAddressPorts, err := api.controllerNodeService.GetAllNoProxyAPIAddressesForAgents(ctx)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	jujuProxySettings := config.JujuProxySettings()
	legacyProxySettings := config.LegacyProxySettings()

	if jujuProxySettings.HasProxySet() {
		jujuProxySettings.AutoNoProxy = proxyAddressPorts
	} else {
		legacyProxySettings.AutoNoProxy = proxyAddressPorts
	}
	result.JujuProxySettings = toParams(jujuProxySettings)
	result.LegacyProxySettings = toParams(legacyProxySettings)

	result.APTProxySettings = toParams(config.AptProxySettings())
	result.AptMirror = config.AptMirror()

	result.SnapProxySettings = toParams(config.SnapProxySettings())
	result.SnapStoreProxyId = config.SnapStoreProxy()
	result.SnapStoreProxyAssertions = config.SnapStoreAssertions()
	result.SnapStoreProxyURL = config.SnapStoreProxyURL()

	return result
}

// ProxyConfig returns the proxy settings for the current model.
func (api *API) ProxyConfig(ctx context.Context, args params.Entities) params.ProxyConfigResults {
	var result params.ProxyConfigResult
	errors, ok := api.authEntities(args)

	if ok {
		result = api.proxyConfig(ctx)
	}

	results := params.ProxyConfigResults{
		Results: make([]params.ProxyConfigResult, len(args.Entities)),
	}
	for i := range args.Entities {
		if errors.Results[i].Error == nil {
			results.Results[i] = result
		}
		results.Results[i].Error = errors.Results[i].Error
	}

	return results
}
