// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/rpc/params"
)

// Create sends a request to create a backup of juju's state.  It
// returns the metadata associated with the resulting backup and a
// filename for download.
func (c *Client) Create(notes string, noDownload bool) (*params.BackupsMetadataResult, error) {
	var result params.BackupsMetadataResult
	args := params.BackupsCreateArgs{
		Notes:      notes,
		NoDownload: noDownload,
	}

	if err := c.facade.FacadeCall(context.TODO(), "Create", args, &result); err != nil {
		return nil, errors.Trace(err)
	}

	return &result, nil
}
