// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
)

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller) *client {
	return &client{base.NewFacadeCaller(caller, "MigrationMinion")}
}

// client implements Client.
type client struct {
	caller base.FacadeCaller
}

// Watch returns a watcher which reports when the status changes for
// the migration for the model associated with the API connection.
func (c *client) Watch() (watcher.MigrationStatusWatcher, error) {
	var result params.NotifyWatchResult
	err := c.caller.FacadeCall("Watch", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewMigrationStatusWatcher(c.caller.RawAPICaller(), result.NotifyWatcherId)
	return w, nil
}
