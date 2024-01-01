// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/proxy"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

const proxyUpdaterFacade = "ProxyUpdater"

// API provides access to the ProxyUpdater API facade.
type API struct {
	tag    names.Tag
	facade base.FacadeCaller
}

// NewAPI returns a new api client facade instance.
func NewAPI(caller base.APICaller, tag names.Tag) (*API, error) {
	if caller == nil {
		return nil, fmt.Errorf("caller is nil")
	}

	if tag == nil {
		return nil, fmt.Errorf("tag is nil")
	}

	return &API{
		facade: base.NewFacadeCaller(caller, proxyUpdaterFacade),
		tag:    tag,
	}, nil
}

// WatchForProxyConfigAndAPIHostPortChanges returns a NotifyWatcher waiting for
// changes in the proxy configuration or API host ports
func (api *API) WatchForProxyConfigAndAPIHostPortChanges() (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: api.tag.String()}},
	}
	err := api.facade.FacadeCall("WatchForProxyConfigAndAPIHostPortChanges", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}

	return newNotifyWatcher(api.facade.RawAPICaller(), result), nil
}

var newNotifyWatcher = apiwatcher.NewNotifyWatcher

func proxySettingsParamToProxySettings(cfg params.ProxyConfig) proxy.Settings {
	return proxy.Settings{
		Http:    cfg.HTTP,
		Https:   cfg.HTTPS,
		Ftp:     cfg.FTP,
		NoProxy: cfg.NoProxy,
	}
}

// ProxyConfiguration contains the various proxy values for the model.
type ProxyConfiguration struct {
	LegacyProxy proxy.Settings
	JujuProxy   proxy.Settings
	APTProxy    proxy.Settings
	SnapProxy   proxy.Settings

	AptMirror string

	SnapStoreProxyId         string
	SnapStoreProxyAssertions string
	SnapStoreProxyURL        string
}

// ProxyConfig returns the proxy settings for the current model.
func (api *API) ProxyConfig() (ProxyConfiguration, error) {
	var empty ProxyConfiguration

	var results params.ProxyConfigResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: api.tag.String()}},
	}
	err := api.facade.FacadeCall("ProxyConfig", args, &results)
	if err != nil {
		return empty, err
	}
	if len(results.Results) != 1 {
		return empty, errors.NotFoundf("ProxyConfig for %q", api.tag)
	}
	result := results.Results[0]
	return ProxyConfiguration{
		LegacyProxy: proxySettingsParamToProxySettings(result.LegacyProxySettings),
		JujuProxy:   proxySettingsParamToProxySettings(result.JujuProxySettings),
		APTProxy:    proxySettingsParamToProxySettings(result.APTProxySettings),
		SnapProxy:   proxySettingsParamToProxySettings(result.SnapProxySettings),

		AptMirror:                result.AptMirror,
		SnapStoreProxyId:         result.SnapStoreProxyId,
		SnapStoreProxyAssertions: result.SnapStoreProxyAssertions,
		SnapStoreProxyURL:        result.SnapStoreProxyURL,
	}, nil
}
