// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// Info implements the API method.
func (c *Client) Info(id string) (*params.BackupsMetadataResult, error) {
	var result params.BackupsMetadataResult
	args := params.BackupsInfoArgs{ID: id}
	if err := c.facade.FacadeCall("Info", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	return &result, nil
}
