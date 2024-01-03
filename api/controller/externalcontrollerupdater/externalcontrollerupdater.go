// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

const Facade = "ExternalControllerUpdater"

// Client provides access to the ExternalControllerUpdater API facade.
type Client struct {
	facade base.FacadeCaller
}

// New creates a new client-side ExternalControllerUpdater facade.
func New(caller base.APICaller, options ...Option) *Client {
	return &Client{base.NewFacadeCaller(caller, Facade, options...)}
}

// WatchExternalControllers watches for the addition and removal of external
// controllers.
func (c *Client) WatchExternalControllers() (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	err := c.facade.FacadeCall(context.TODO(), "WatchExternalControllers", nil, &results)
	if err != nil {
		return nil, err
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}
	result := results.Results[0]
	if result.Error != nil {
		err := apiservererrors.RestoreError(result.Error)
		return nil, errors.Trace(err)
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
	err := c.facade.FacadeCall(context.TODO(), "ExternalControllerInfo", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		err := apiservererrors.RestoreError(result.Error)
		return nil, errors.Trace(err)
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
	err := c.facade.FacadeCall(context.TODO(), "SetExternalControllerInfo", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}
