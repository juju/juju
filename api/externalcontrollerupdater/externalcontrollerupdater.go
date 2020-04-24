// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/watcher"
)

const Facade = "ExternalControllerUpdater"

// Client provides access to the ExternalControllerUpdater API facade.
type Client struct {
	facade base.FacadeCaller
}

// New creates a new client-side ExternalControllerUpdater facade.
func New(caller base.APICaller) *Client {
	return &Client{base.NewFacadeCaller(caller, Facade)}
}

// WatchExternalControllers watches for the addition and removal of external
// controllers.
func (c *Client) WatchExternalControllers() (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	err := c.facade.FacadeCall("WatchExternalControllers", nil, &results)
	if err != nil {
		return nil, err
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// ExternalControllerInfo returns the info for the external controller with the specified UUID.
func (c *Client) ExternalControllerInfo(controllerUUID string) (*crossmodel.ControllerInfo, error) {
	if !names.IsValidController(controllerUUID) {
		return nil, errors.NotValidf("controller UUID %q", controllerUUID)
	}
	controllerTag := names.NewControllerTag(controllerUUID)
	args := params.Entities{[]params.Entity{{
		Tag: controllerTag.String(),
	}}}
	var results params.ExternalControllerInfoResults
	err := c.facade.FacadeCall("ExternalControllerInfo", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return &crossmodel.ControllerInfo{
		ControllerTag: controllerTag,
		Alias:         result.Result.Alias,
		Addrs:         result.Result.Addrs,
		CACert:        result.Result.CACert,
	}, nil
}

// SetExternalControllerInfo saves the given controller info.
func (c *Client) SetExternalControllerInfo(info crossmodel.ControllerInfo) error {
	var results params.ErrorResults
	args := params.SetExternalControllersInfoParams{
		Controllers: []params.SetExternalControllerInfoParams{{
			Info: params.ExternalControllerInfo{
				ControllerTag: info.ControllerTag.String(),
				Alias:         info.Alias,
				Addrs:         info.Addrs,
				CACert:        info.CACert,
			},
		}},
	}
	err := c.facade.FacadeCall("SetExternalControllerInfo", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}
