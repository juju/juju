// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// Create sends a request to create a backup of juju's state.  It
// returns the metadata associated with the resulting backup and a
// filename for download.
func (c *Client) Create(notes string, keepCopy, noDownload bool) (*params.BackupsCreateResult, error) {
	var result params.BackupsCreateResult
	args := params.BackupsCreateArgs{
		Notes:      notes,
		KeepCopy:   keepCopy,
		NoDownload: noDownload,
	}
	if err := c.facade.FacadeCall("CreateBackup", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	return &result, nil
}

// CreateDeprecated sends a request to create a backup of juju's state.  It
// returns the metadata associated with the resulting backup.
//
// NOTE(hml) this exists only for backwards compatibility, for API facade
// versions 1; clients should prefer its successor, Create, above.
//
// TODO(hml) 2018-05-02
// Drop this in Juju 3.0.
func (c *Client) CreateDeprecated(notes string) (*params.BackupsMetadataResult, error) {
	var result params.BackupsMetadataResult
	args := params.BackupsCreateArgs{Notes: notes}
	if err := c.facade.FacadeCall("Create", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	return &result, nil
}
