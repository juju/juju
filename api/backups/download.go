// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// Download implements the API method.
func (c *Client) Download(id string) (*params.BackupsDownloadResult, error) {
	var result params.BackupsDownloadResult
	args := params.BackupsDownloadArgs{ID: id}
	if err := c.facade.FacadeCall("DownloadDirect", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	return &result, nil
}
