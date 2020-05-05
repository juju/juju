// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/proxy"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// ProxyUpdaterV1 defines the pubic methods for the v1 facade.
type ProxyUpdaterV1 interface {
	ProxyConfig(args params.Entities) params.ProxyConfigResultsV1
	WatchForProxyConfigAndAPIHostPortChanges(args params.Entities) params.NotifyWatchResults
}

var _ ProxyUpdaterV1 = (*APIv1)(nil)

// ProxyUpdaterV2 defines the pubic methods for the v2 facade.
type ProxyUpdaterV2 interface {
	ProxyConfig(args params.Entities) params.ProxyConfigResults
	WatchForProxyConfigAndAPIHostPortChanges(args params.Entities) params.NotifyWatchResults
}

var _ ProxyUpdaterV2 = (*APIv2)(nil)

// NewFacadeV1 provides the signature required for facade registration
// and creates a v1 facade.
func NewFacadeV1(ctx facade.Context) (*APIv1, error) {
	api, err := NewFacadeV2(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv1{api}, nil
}

// NewFacadeV2 provides the signature required for facade registration
// and creates a v2 facade.
func NewFacadeV2(ctx facade.Context) (*APIv2, error) {
	api, err := newFacadeBase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}

func newFacadeBase(ctx facade.Context) (*APIBase, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, err
	}
	return NewAPIBase(
		&stateShim{st: st, m: m},
		ctx.Resources(),
		ctx.Auth(),
	)
}

// APIv1 provides the ProxyUpdater version 1 facade.
type APIv1 struct {
	*APIv2
}

// APIv2 provides the ProxyUpdater version 2 facade.
type APIv2 struct {
	*APIBase
}

type APIBase struct {
	backend    Backend
	resources  facade.Resources
	authorizer facade.Authorizer
}

// Backend defines the state methods this facade needs, so they can be
// mocked for testing.
type Backend interface {
	ModelConfig() (*config.Config, error)
	APIHostPortsForAgents() ([]network.SpaceHostPorts, error)
	WatchAPIHostPortsForAgents() state.NotifyWatcher
	WatchForModelConfigChanges() state.NotifyWatcher
}

// NewAPIBase creates a new server-side API facade with the given Backing.
func NewAPIBase(backend Backend, resources facade.Resources, authorizer facade.Authorizer) (*APIBase, error) {
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent() || authorizer.AuthApplicationAgent()) {
		return nil, common.ErrPerm
	}
	return &APIBase{
		backend:    backend,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

func (api *APIBase) oneWatch() params.NotifyWatchResult {
	var result params.NotifyWatchResult

	watch := common.NewMultiNotifyWatcher(
		api.backend.WatchForModelConfigChanges(),
		api.backend.WatchAPIHostPortsForAgents())

	if _, ok := <-watch.Changes(); ok {
		result = params.NotifyWatchResult{
			NotifyWatcherId: api.resources.Register(watch),
		}
	} else {
		result.Error = common.ServerError(watcher.EnsureErr(watch))
	}
	return result
}

// WatchForProxyConfigAndAPIHostPortChanges watches for cleanups to be perfomed in state
func (api *APIBase) WatchForProxyConfigAndAPIHostPortChanges(args params.Entities) params.NotifyWatchResults {
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

func (api *APIBase) authEntities(args params.Entities) (params.ErrorResults, bool) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}

	var ok bool

	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if !api.authorizer.AuthOwner(tag) {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		ok = true
	}
	return result, ok
}

func (api *APIBase) proxyConfig() params.ProxyConfigResult {
	var result params.ProxyConfigResult
	config, err := api.backend.ModelConfig()
	if err != nil {
		result.Error = common.ServerError(err)
		return result
	}

	apiHostPorts, err := api.backend.APIHostPortsForAgents()
	if err != nil {
		result.Error = common.ServerError(err)
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

	result.SnapProxySettings = toParams(config.SnapProxySettings())
	result.SnapStoreProxyId = config.SnapStoreProxy()
	result.SnapStoreProxyAssertions = config.SnapStoreAssertions()
	result.SnapStoreProxyURL = config.SnapStoreProxyURL()

	return result
}

// ProxyConfig returns the proxy settings for the current model.
func (api *APIBase) ProxyConfig(args params.Entities) params.ProxyConfigResults {
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

// ProxyConfig returns the proxy settings for the current model.
func (api *APIv1) ProxyConfig(args params.Entities) params.ProxyConfigResultsV1 {
	var result params.ProxyConfigResultV1
	errors, ok := api.authEntities(args)

	if ok {
		v2 := api.proxyConfig()
		result = params.ProxyConfigResultV1{
			ProxySettings:    v2.LegacyProxySettings,
			APTProxySettings: v2.APTProxySettings,
		}
	}

	results := params.ProxyConfigResultsV1{
		Results: make([]params.ProxyConfigResultV1, len(args.Entities)),
	}
	for i := range args.Entities {
		if errors.Results[i].Error == nil {
			results.Results[i] = result
		}
		results.Results[i].Error = errors.Results[i].Error
	}

	return results
}
