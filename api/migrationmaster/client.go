// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/watcher"
)

// Client describes the client side API for the MigrationMaster facade
// (used by the migration master worker).
type Client interface {
	// Watch returns a watcher which reports when a migration is
	// active for the model associated with the API connection.
	Watch() (watcher.MigrationMasterWatcher, error)

	// SetPhase updates the phase of the currently active model
	// migration.
	SetPhase(migration.Phase) error

	// Export returns a serialized representation of the model
	// associated with the API connection.
	Export() ([]byte, error)
}

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller) Client {
	return &client{base.NewFacadeCaller(caller, "MigrationMaster")}
}

// client implements Client.
type client struct {
	caller base.FacadeCaller
}

// Watch implements Client.
func (c *client) Watch() (watcher.MigrationMasterWatcher, error) {
	var result params.NotifyWatchResult
	err := c.caller.FacadeCall("Watch", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewMigrationMasterWatcher(c.caller.RawAPICaller(), result.NotifyWatcherId)
	return w, nil
}

// SetPhase implements Client.
func (c *client) SetPhase(phase migration.Phase) error {
	args := params.SetMigrationPhaseArgs{
		Phase: phase.String(),
	}
	return c.caller.FacadeCall("SetPhase", args, nil)
}

// Export implements Client.
func (c *client) Export() ([]byte, error) {
	var serialized params.SerializedModel
	err := c.caller.FacadeCall("Export", nil, &serialized)
	if err != nil {
		return nil, err
	}
	return serialized.Bytes, nil
}
