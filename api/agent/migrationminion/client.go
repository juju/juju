// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller, options ...Option) *Client {
	return &Client{base.NewFacadeCaller(caller, "MigrationMinion", options...)}
}

type Client struct {
	caller base.FacadeCaller
}

// Watch returns a watcher which reports when the status changes for
// the migration for the model associated with the API connection.
func (c *Client) Watch() (watcher.MigrationStatusWatcher, error) {
	var result params.NotifyWatchResult
	err := c.caller.FacadeCall(context.TODO(), "Watch", nil, &result)
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
	err := c.caller.FacadeCall(context.TODO(), "Report", args, nil)
	return errors.Trace(err)
}
