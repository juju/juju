// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/proxy"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// ProxyUpdaterV2 defines the pubic methods for the v2 facade.
type ProxyUpdaterV2 interface {
	ProxyConfig(args params.Entities) params.ProxyConfigResults
	WatchForProxyConfigAndAPIHostPortChanges(args params.Entities) params.NotifyWatchResults
}

var _ ProxyUpdaterV2 = (*API)(nil)

// newFacadeBase provides the signature required for facade registration
// and creates a v2 facade.
func newFacadeBase(ctx facade.Context) (*API, error) {
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, err
	}
	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewAPIV2(
		systemState,
		model,
		ctx.Resources(),
		ctx.Auth(),
	)
}

// API provides the ProxyUpdater version 2 facade.
type API struct {
	backend    Backend
	controller ControllerBackend
	resources  facade.Resources
	authorizer facade.Authorizer
}

// Backend defines the model state methods this facade needs,
// so they can be mocked for testing.
type Backend interface {
	ModelConfig() (*config.Config, error)
	WatchForModelConfigChanges() state.NotifyWatcher
}

// ControllerBackend defines the controller state methods this facade needs,
// so they can be mocked for testing.
type ControllerBackend interface {
	APIHostPortsForAgents() ([]network.SpaceHostPorts, error)
	WatchAPIHostPortsForAgents() state.NotifyWatcher
}

// NewAPIV2 creates a new server-side API facade with the given Backing.
func NewAPIV2(controller ControllerBackend, backend Backend, resources facade.Resources, authorizer facade.Authorizer) (*API, error) {
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent() || authorizer.AuthApplicationAgent() || authorizer.AuthModelAgent()) {
		return nil, apiservererrors.ErrPerm
	}
	return &API{
		backend:    backend,
		controller: controller,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

func (api *API) oneWatch() params.NotifyWatchResult {
	var result params.NotifyWatchResult

	watch := common.NewMultiNotifyWatcher(
		api.backend.WatchForModelConfigChanges(),
		api.controller.WatchAPIHostPortsForAgents())

	if _, ok := <-watch.Changes(); ok {
		result = params.NotifyWatchResult{
			NotifyWatcherId: api.resources.Register(watch),
		}
	} else {
		result.Error = apiservererrors.ServerError(watcher.EnsureErr(watch))
	}
	return result
}

// WatchForProxyConfigAndAPIHostPortChanges watches for changes to the proxy and api host port settings.
func (api *API) WatchForProxyConfigAndAPIHostPortChanges(args params.Entities) params.NotifyWatchResults {
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	errors, _ := api.authEntities(args)

	for i := range args.Entities {
		if errors.Results[i].Error == nil {
			results.Results[i] = api.oneWatch()
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

func (api *API) proxyConfig() params.ProxyConfigResult {
	var result params.ProxyConfigResult
	config, err := api.backend.ModelConfig()
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	apiHostPorts, err := api.controller.APIHostPortsForAgents()
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	jujuProxySettings := config.JujuProxySettings()
	legacyProxySettings := config.LegacyProxySettings()

	if jujuProxySettings.HasProxySet() {
		jujuProxySettings.AutoNoProxy = network.APIHostPortsToNoProxyString(apiHostPorts)
	} else {
		legacyProxySettings.AutoNoProxy = network.APIHostPortsToNoProxyString(apiHostPorts)
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
func (api *API) ProxyConfig(args params.Entities) params.ProxyConfigResults {
	var result params.ProxyConfigResult
	errors, ok := api.authEntities(args)

	if ok {
		result = api.proxyConfig()
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
