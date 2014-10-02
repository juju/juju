// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// Create sends a request to create a backup of juju's state.  It
// returns the metadata associated with the resulting backup.
func (c *Client) Create(notes string) (*params.BackupsMetadataResult, error) {
	var result params.BackupsMetadataResult
	args := params.BackupsCreateArgs{Notes: notes}
	if err := c.facade.FacadeCall("Create", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	return &result, nil
}
