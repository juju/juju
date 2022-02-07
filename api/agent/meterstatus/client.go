// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package meterstatus contains an implementation of the api facade to
// watch the meter status of a unit for changes and return the current meter status.
package meterstatus

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
)

// MeterStatusClient defines the methods on the MeterStatus API end point.
type MeterStatusClient interface {
	// MeterStatus returns the meter status and additional information for the
	// API client.
	MeterStatus() (string, string, error)
	// WatchMeterStatus returns a watcher for observing changes to the unit's meter
	// status.
	WatchMeterStatus() (watcher.NotifyWatcher, error)
}

// NewClient creates a new client for accessing the MeterStatus API.
func NewClient(caller base.APICaller, tag names.UnitTag) MeterStatusClient {
	return &Client{
		facade: base.NewFacadeCaller(caller, "MeterStatus"),
		tag:    tag,
	}
}

var _ MeterStatusClient = (*Client)(nil)

// Client provides access to the meter status API.
type Client struct {
	facade base.FacadeCaller
	tag    names.UnitTag
}

// MeterStatus is part of the MeterStatusClient interface.
func (c *Client) MeterStatus() (statusCode, statusInfo string, rErr error) {
	var results params.MeterStatusResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: c.tag.String()}},
	}
	err := c.facade.FacadeCall("GetMeterStatus", args, &results)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return "", "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", "", errors.Trace(result.Error)
	}
	return result.Code, result.Info, nil
}

// WatchMeterStatus is part of the MeterStatusClient interface.
func (c *Client) WatchMeterStatus() (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: c.tag.String()}},
	}
	err := c.facade.FacadeCall("WatchMeterStatus", args, &results)
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
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}
