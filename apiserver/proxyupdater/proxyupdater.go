// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"strings"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/names"
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/set"
)

// Backend defines the state methods this facade needs, so they can be
// mocked for testing.
type Backend interface {
	ModelConfig() (*config.Config, error)
	APIHostPorts() ([][]network.HostPort, error)
	WatchAPIHostPorts() state.NotifyWatcher
	WatchForModelConfigChanges() state.NotifyWatcher
}

type ProxyUpdaterAPI struct {
	backend    Backend
	resources  *common.Resources
	authorizer common.Authorizer
}

// NewAPIWithBacking creates a new server-side API facade with the given Backing.
func NewAPIWithBacking(st Backend, resources *common.Resources, authorizer common.Authorizer) (*ProxyUpdaterAPI, error) {
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent()) {
		return &ProxyUpdaterAPI{}, common.ErrPerm
	}
	return &ProxyUpdaterAPI{
		backend:    st,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

func (api *ProxyUpdaterAPI) oneWatch() params.NotifyWatchResult {
	var result params.NotifyWatchResult

	watch := common.NewMultiNotifyWatcher(
		api.backend.WatchForModelConfigChanges(),
		api.backend.WatchAPIHostPorts())

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
func (api *ProxyUpdaterAPI) WatchForProxyConfigAndAPIHostPortChanges(args params.Entities) params.NotifyWatchResults {
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

func proxyUtilsSettingsToProxySettingsParam(settings proxy.Settings) params.ProxyConfig {
	return params.ProxyConfig{
		HTTP:    settings.Http,
		HTTPS:   settings.Https,
		FTP:     settings.Ftp,
		NoProxy: settings.NoProxy,
	}
}

func (api *ProxyUpdaterAPI) authEntities(args params.Entities) (params.ErrorResults, bool) {
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

func (api *ProxyUpdaterAPI) proxyConfig() params.ProxyConfigResult {
	var result params.ProxyConfigResult
	env, err := api.backend.ModelConfig()
	if err != nil {
		result.Error = common.ServerError(err)
		return result
	}

	apiHostPorts, err := api.backend.APIHostPorts()
	if err != nil {
		result.Error = common.ServerError(err)
		return result
	}

	result.ProxySettings = proxyUtilsSettingsToProxySettingsParam(env.ProxySettings())
	result.APTProxySettings = proxyUtilsSettingsToProxySettingsParam(env.AptProxySettings())

	var noProxy []string
	if result.ProxySettings.NoProxy != "" {
		noProxy = strings.Split(result.ProxySettings.NoProxy, ",")
	}

	noProxySet := set.NewStrings(noProxy...)
	for _, host := range apiHostPorts {
		for _, hp := range host {
			noProxySet.Add(hp.Address.Value)
		}
	}
	result.ProxySettings.NoProxy = strings.Join(noProxySet.SortedValues(), ",")

	return result
}

// ProxyConfig returns the proxy settings for the current environment
func (api *ProxyUpdaterAPI) ProxyConfig(args params.Entities) params.ProxyConfigResults {
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
