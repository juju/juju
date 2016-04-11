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

// Client describes the client side API for the MigrationMinion facade
// (used by the migration minion worker).
type Client interface {
	// Watch returns a watcher which reports when the status changes
	// for the migration for the model associated with the API
	// connection.
	Watch() (watcher.MigrationStatusWatcher, error)
}

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller) Client {
	return &client{base.NewFacadeCaller(caller, "MigrationMinion")}
}

// client implements Client.
type client struct {
	caller base.FacadeCaller
}

// Watch implements Client.
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
