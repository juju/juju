// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"io/ioutil"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// Upload sends the backup archive to storage.
func (c *Client) Upload(archive io.Reader, meta params.BackupsMetadataResult) (*params.BackupsMetadataResult, error) {
	data, err := ioutil.ReadAll(archive)
	if err != nil {
		return nil, errors.Trace(err)
	}

	args := params.BackupsUploadArgs{
		Data:     data,
		Metadata: meta,
	}

	var result params.BackupsMetadataResult
	if err := c.facade.FacadeCall("UploadDirect", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	return &result, nil
}
