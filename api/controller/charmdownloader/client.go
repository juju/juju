// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client allows access to the charmdownloader API.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the charmdownloader API.
func NewClient(caller base.APICaller, options ...Option) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CharmDownloader", options...)
	return &Client{facade: facadeCaller}
}

// WatchApplicationsWithPendingCharms emits the application names that
// reference a charm that is pending to be downloaded.
func (c *Client) WatchApplicationsWithPendingCharms() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := c.facade.FacadeCall(context.TODO(), "WatchApplicationsWithPendingCharms", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, result.Error
	}

	return apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result), nil
}

// DownloadApplicationCharms iterates the list of provided applications and
// downloads any referenced charms that have not yet been persisted to the
// blob store.
func (c *Client) DownloadApplicationCharms(applications []names.ApplicationTag) error {
	args := params.Entities{
		Entities: make([]params.Entity, len(applications)),
	}
	for i, app := range applications {
		args.Entities[i].Tag = app.String()
	}

	var res params.ErrorResults
	err := c.facade.FacadeCall(context.TODO(), "DownloadApplicationCharms", args, &res)
	if err != nil {
		return errors.Trace(err)
	}

	return res.Combine()
}
