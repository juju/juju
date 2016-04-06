// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/errors"
	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
	"github.com/juju/names"
	"github.com/juju/utils/proxy"
)

const proxyUpdaterFacade = "ProxyUpdater"

// API provides access to the ProxyUpdater API facade.
type API struct {
	tag    names.Tag
	facade base.FacadeCaller
}

// NewAPI returns a new api client facade instance.
func NewAPI(caller base.APICaller, tag names.Tag) *API {
	if caller == nil {
		panic("caller is nil")
	}
	return &API{
		facade: base.NewFacadeCaller(caller, proxyUpdaterFacade),
		tag:    tag,
	}
}

// WatchForProxyConfigAndAPIHostPortChanges returns a NotifyWatcher waiting for
// changes in the proxy configuration or API host ports
func (api *API) WatchForProxyConfigAndAPIHostPortChanges() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	args := params.Entities{
		Entities: []params.Entity{{Tag: api.tag.String()}},
	}
	err := api.facade.FacadeCall("WatchForProxyConfigAndAPIHostPortChanges", args, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}

	return apiwatcher.NewNotifyWatcher(api.facade.RawAPICaller(), result), nil
}

func proxySettingsParamToProxySettings(cfg params.ProxyConfig) proxy.Settings {
	return proxy.Settings{
		Http:    cfg.HTTP,
		Https:   cfg.HTTPS,
		Ftp:     cfg.FTP,
		NoProxy: cfg.NoProxy,
	}
}

// ProxyConfig returns the proxy settings for the current environment
func (api *API) ProxyConfig() (proxySettings, APTProxySettings proxy.Settings, err error) {
	var results params.ProxyConfigResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: api.tag.String()}},
	}
	err = api.facade.FacadeCall("ProxyConfig", args, &results)
	if err != nil {
		return proxySettings, APTProxySettings, err
	}
	if len(results.Results) != 1 {
		return proxySettings, APTProxySettings, errors.NotFoundf("ProxyConfig for %q not found", api.tag)
	}
	result := results.Results[0]
	proxySettings = proxySettingsParamToProxySettings(result.ProxySettings)
	APTProxySettings = proxySettingsParamToProxySettings(result.APTProxySettings)
	return proxySettings, APTProxySettings, err
}
