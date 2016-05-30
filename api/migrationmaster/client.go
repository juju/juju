// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/watcher"
)

// MigrationStatus returns the details for a migration as needed by
// the migration master worker.
type MigrationStatus struct {
	ModelUUID  string
	Attempt    int
	Phase      migration.Phase
	TargetInfo migration.TargetInfo
}

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller) *Client {
	return &Client{base.NewFacadeCaller(caller, "MigrationMaster")}
}

// Client describes the client side API for the MigrationMaster facade
// (used by the migrationmaster worker).
type Client struct {
	caller base.FacadeCaller
}

// Watch returns a watcher which reports when a migration is active
// for the model associated with the API connection.
func (c *Client) Watch() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := c.caller.FacadeCall("Watch", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewNotifyWatcher(c.caller.RawAPICaller(), result)
	return w, nil
}

// GetMigrationStatus returns the details and progress of the latest
// model migration.
func (c *Client) GetMigrationStatus() (MigrationStatus, error) {
	var empty MigrationStatus
	var status params.FullMigrationStatus
	err := c.caller.FacadeCall("GetMigrationStatus", nil, &status)
	if err != nil {
		return empty, errors.Trace(err)
	}

	modelTag, err := names.ParseModelTag(status.Spec.ModelTag)
	if err != nil {
		return empty, errors.Annotatef(err, "parsing model tag")
	}

	phase, ok := migration.ParsePhase(status.Phase)
	if !ok {
		return empty, errors.New("unable to parse phase")
	}

	target := status.Spec.TargetInfo
	controllerTag, err := names.ParseModelTag(target.ControllerTag)
	if err != nil {
		return empty, errors.Annotatef(err, "parsing controller tag")
	}

	authTag, err := names.ParseUserTag(target.AuthTag)
	if err != nil {
		return empty, errors.Annotatef(err, "unable to parse auth tag")
	}

	return MigrationStatus{
		ModelUUID: modelTag.Id(),
		Attempt:   status.Attempt,
		Phase:     phase,
		TargetInfo: migration.TargetInfo{
			ControllerTag: controllerTag,
			Addrs:         target.Addrs,
			CACert:        target.CACert,
			AuthTag:       authTag,
			Password:      target.Password,
		},
	}, nil
}

// SetPhase updates the phase of the currently active model migration.
func (c *Client) SetPhase(phase migration.Phase) error {
	args := params.SetMigrationPhaseArgs{
		Phase: phase.String(),
	}
	return c.caller.FacadeCall("SetPhase", args, nil)
}

// Export returns a serialized representation of the model associated
// with the API connection.
func (c *Client) Export() ([]byte, error) {
	var serialized params.SerializedModel
	err := c.caller.FacadeCall("Export", nil, &serialized)
	if err != nil {
		return nil, err
	}
	return serialized.Bytes, nil
}

// Reap removes the documents for the model associated with the API
// connection.
func (c *Client) Reap() error {
	return c.caller.FacadeCall("Reap", nil, nil)
}
