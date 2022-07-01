// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/v2/rpc/params"
)

// The following methods are for dealing with backups stored on the controller.
// This is something we have not supported for a long time, and for the past 4 years,
// the default has been to NOT store the backups on the controller.
// TODO(wallyworld) - remove in juju 3.

var notSupported = errors.NotSupportedf("backups stored in the controller database")

// Info provides the implementation of the API method.
func (a *API) Info(args params.BackupsInfoArgs) (params.BackupsMetadataResult, error) {
	return params.BackupsMetadataResult{}, notSupported
}

// List provides the implementation of the API method.
func (a *API) List(args params.BackupsListArgs) (params.BackupsListResult, error) {
	return params.BackupsListResult{}, notSupported
}

// Remove deletes the backups defined by ID from the database.
func (a *APIv2) Remove(args params.BackupsRemoveArgs) (params.ErrorResults, error) {
	return params.ErrorResults{}, notSupported
}

// Restore implements the server side of Backups.Restore.
func (a *API) Restore(p params.RestoreArgs) error {
	return errors.New("restore is now provided by the standalone juju-restore utility")
}
