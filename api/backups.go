// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// BackupsClient wraps the backups API for the client.
type BackupsClient struct {
	Client
}

// Backups returns the backups-specific portion of the client.
func (c *Client) Backups() *BackupsClient {
	client := c.st.newClient("Backups")
	return &BackupsClient{Client: *client}
}

// Create sends a request to create a backup of juju's state.  It
// returns the metadata associated with the resulting backup.
func (c *BackupsClient) Create(notes string) (*params.BackupsMetadataResult, error) {
	var result params.BackupsMetadataResult
	args := params.BackupsCreateArgs{Notes: notes}
	if err := c.facade.FacadeCall("Create", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	return &result, nil
}
