// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
)

const proxyUpdasterFacade = "ProxyUpdater"

// API provides access to the ProxyUpdater API facade.
type API struct {
	facade base.FacadeCaller
}

// NewAPI creates a new client-side ProxyUpdater facade.
func NewAPI(caller base.APICaller) *API {
	if caller == nil {
		panic("caller is nil")
	}
	return &API{
		facade: base.NewFacadeCaller(caller, proxyUpdasterFacade),
	}
}

// WatchForProxyConfigAndAPIHostPortChanges returns a NotifyWatcher waiting for
// changes in the proxy configuration or API host ports
func (api *API) WatchForProxyConfigAndAPIHostPortChanges() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := api.facade.FacadeCall("WatchForEnvironConfigAndAPIHostPortChanges", nil, &result)
	if err != nil {
		return nil, err
	}
	return watcher.NewNotifyWatcher(api.facade.RawAPICaller(), result), nil
}

// EnvironConfig returns the current environment configuration.
func (api *API) EnvironConfig() (*config.Config, error) {
	var result params.EnvironConfigResult
	err := api.facade.FacadeCall("EnvironConfig", nil, &result)
	if err != nil {
		return nil, err
	}
	conf, err := config.New(config.NoDefaults, result.Config)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// APIHostPorts returns the host/port addresses of the API servers.
func (api *API) APIHostPorts() ([][]network.HostPort, error) {
	var result params.APIHostPortsResult
	err := api.facade.FacadeCall("APIHostPorts", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.NetworkHostsPorts(), nil
}
