// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Client allows access to the charmdownloader API.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the charmdownloader API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CharmDownloader")
	return &Client{facade: facadeCaller}
}

// WatchApplicationsWithPendingCharms emits the application names that
// reference a charm that is pending to be downloaded.
func (c *Client) WatchApplicationsWithPendingCharms() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := c.facade.FacadeCall("WatchApplicationsWithPendingCharms", nil, &result)
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
	err := c.facade.FacadeCall("DownloadApplicationCharms", args, &res)
	if err != nil {
		return errors.Trace(err)
	}

	return res.Combine()
}
