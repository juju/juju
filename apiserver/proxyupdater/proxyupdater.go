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

// API defines the methods the ProxyUpdater API facade implements.
type API interface {
	WatchForProxyConfigAndAPIHostPortChanges(_ params.Entities) params.NotifyWatchResult
	ProxyConfig(_ params.Entities) params.ProxyConfigResults
}

// Backend defines the state methods this facade needs, so they can be
// mocked for testing.
type Backend interface {
	EnvironConfig() (*config.Config, error)
	APIHostPorts() ([][]network.HostPort, error)
	WatchAPIHostPorts() state.NotifyWatcher
	WatchForModelConfigChanges() state.NotifyWatcher
}

type proxyUpdaterAPI struct {
	backend    Backend
	resources  *common.Resources
	authorizer common.Authorizer
}

// NewAPIWithBacking creates a new server-side API facade with the given Backing.
func NewAPIWithBacking(st Backend, resources *common.Resources, authorizer common.Authorizer) (API, error) {
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent()) {
		return nil, common.ErrPerm
	}
	return &proxyUpdaterAPI{
		backend:    st,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// WatchChanges watches for cleanups to be perfomed in state
func (api *proxyUpdaterAPI) WatchForProxyConfigAndAPIHostPortChanges(_ params.Entities) params.NotifyWatchResult {
	watch := common.NewMultiNotifyWatcher(
		api.backend.WatchForModelConfigChanges(),
		api.backend.WatchAPIHostPorts())
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: api.resources.Register(watch),
		}
	}
	return params.NotifyWatchResult{
		Error: common.ServerError(watcher.EnsureErr(watch)),
	}
}

func proxyUtilsSettingsToProxySettingsParam(settings proxy.Settings) params.ProxyConfig {
	return params.ProxyConfig{
		HTTP:    settings.Http,
		HTTPS:   settings.Https,
		FTP:     settings.Ftp,
		NoProxy: settings.NoProxy,
	}
}

func (api *proxyUpdaterAPI) authEntities(args params.Entities) params.ErrorResults {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}

	for i, entity := range args.Entities {
		_ = i
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if !api.authorizer.AuthOwner(tag) {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result
}

// ProxyConfig returns the proxy settings for the current environment
func (api *proxyUpdaterAPI) ProxyConfig(args params.Entities) params.ProxyConfigResults {
	var result params.ProxyConfigResult
	errors := api.authEntities(args)
	_ = errors

	env, err := api.backend.EnvironConfig()
	if err != nil {
		result.Error = common.ServerError(err)
		return params.ProxyConfigResults{}
	}

	apiHostPorts, err := api.backend.APIHostPorts()
	if err != nil {
		result.Error = common.ServerError(err)
		return params.ProxyConfigResults{}
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

	return params.ProxyConfigResults{
		Results: []params.ProxyConfigResult{result},
	}
}
