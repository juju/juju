// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
)

const proxyUpdaterFacade = "ProxyUpdater"

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
		facade: base.NewFacadeCaller(caller, proxyUpdaterFacade),
	}
}

// WatchForProxyConfigAndAPIHostPortChanges returns a NotifyWatcher waiting for
// changes in the proxy configuration or API host ports
func (api *API) WatchForProxyConfigAndAPIHostPortChanges() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := api.facade.FacadeCall("WatchForProxyConfigAndAPIHostPortChanges", nil, &result)
	if err != nil {
		return nil, err
	}
	return watcher.NewNotifyWatcher(api.facade.RawAPICaller(), result), nil
}

// ProxyConfig returns the current environment configuration.
func (api *API) ProxyConfig() (params.ProxyConfigResult, error) {
	var result params.ProxyConfigResult
	err := api.facade.FacadeCall("ProxyConfig", nil, &result)
	return result, err
}
