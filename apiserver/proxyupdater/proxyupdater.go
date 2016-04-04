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
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/set"
)

// API defines the methods the ProxyUpdater API facade implements.
type API interface {
	WatchForProxyConfigAndAPIHostPortChanges() (params.NotifyWatchResult, error)
	ProxyConfig() (params.ProxyConfigResult, error)
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
func (api *proxyUpdaterAPI) WatchForProxyConfigAndAPIHostPortChanges() (params.NotifyWatchResult, error) {
	watch := common.NewMultiNotifyWatcher(
		api.backend.WatchForModelConfigChanges(),
		api.backend.WatchAPIHostPorts())
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: api.resources.Register(watch),
		}, nil
	}
	return params.NotifyWatchResult{
		Error: common.ServerError(watcher.EnsureErr(watch)),
	}, watcher.EnsureErr(watch)
}

func proxyUtilsSettingsToProxySettingsParam(settings proxy.Settings) params.ProxyConfig {
	return params.ProxyConfig{
		HTTP:    settings.Http,
		HTTPS:   settings.Https,
		FTP:     settings.Ftp,
		NoProxy: settings.NoProxy,
	}
}

func (api *proxyUpdaterAPI) ProxyConfig() (params.ProxyConfigResult, error) {
	var cfg params.ProxyConfigResult
	env, err := api.backend.EnvironConfig()
	if err != nil {
		cfg.Error = common.ServerError(err)
		return cfg, err
	}

	apiHostPorts, err := api.backend.APIHostPorts()
	if err != nil {
		cfg.Error = common.ServerError(err)
		return cfg, err
	}

	cfg.ProxySettings = proxyUtilsSettingsToProxySettingsParam(env.ProxySettings())
	cfg.APTProxySettings = proxyUtilsSettingsToProxySettingsParam(env.AptProxySettings())
	var noProxy []string
	if cfg.ProxySettings.NoProxy != "" {
		noProxy = strings.Split(cfg.ProxySettings.NoProxy, ",")
	}

	noProxySet := set.NewStrings(noProxy...)

	for _, host := range apiHostPorts {
		for _, hp := range host {
			noProxySet.Add(hp.Address.Value)
		}
	}

	cfg.ProxySettings.NoProxy = strings.Join(noProxySet.SortedValues(), ",")
	return cfg, nil
}
