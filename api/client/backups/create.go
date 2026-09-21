// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/rpc/params"
)

// Create sends a request to create a backup of juju's state. It
// returns the metadata associated with the resulting backup, with
// the archive contents inlined in the result.
func (c *Client) Create(ctx context.Context, notes string) (*params.BackupsMetadataResult, error) {
	var result params.BackupsMetadataResult
	args := params.BackupsCreateArgs{
		Notes: notes,
	}

	if err := c.facade.FacadeCall(ctx, "Create", args, &result); err != nil {
		return nil, errors.Trace(err)
	}

	return &result, nil
}
