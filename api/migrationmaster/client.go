// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
)

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller) *Client {
	return &Client{base.NewFacadeCaller(caller, "MigrationMaster")}
}

// Client provides the client side API for the MigrationMaster facade
// (to be used by the migration master worker).
type Client struct {
	caller base.FacadeCaller
}

// Watch returns a watcher which reports when a migration is active
// for the model associated with the API connection.
func (c *Client) Watch() (watcher.MigrationMasterWatcher, error) {
	var result params.NotifyWatchResult
	err := c.caller.FacadeCall("Watch", nil, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewMigrationMasterWatcher(c.caller.RawAPICaller(), result.NotifyWatcherId)
	return w, nil
}
