// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/watcher"
)

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller) *Client {
	return &Client{base.NewFacadeCaller(caller, "MigrationMinion")}
}

type Client struct {
	caller base.FacadeCaller
}

// Watch returns a watcher which reports when the status changes for
// the migration for the model associated with the API connection.
func (c *Client) Watch() (watcher.MigrationStatusWatcher, error) {
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

// Report allows a migration minion to report if it successfully
// completed its activities for a given migration phase.
func (c *Client) Report(migrationId string, phase migration.Phase, success bool) error {
	args := params.MinionReport{
		MigrationId: migrationId,
		Phase:       phase.String(),
		Success:     success,
	}
	err := c.caller.FacadeCall("Report", args, nil)
	return errors.Trace(err)
}
