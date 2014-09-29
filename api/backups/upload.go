// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"io/ioutil"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// Info implements the API method.
func (c *Client) Upload(archive io.ReadCloser, meta params.BackupsMetadataResult) (*params.BackupsMetadataResult, error) {
	defer archive.Close()

	data, err := ioutil.ReadAll(archive)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result params.BackupsMetadataResult
	args := params.BackupsUploadArgs{
		Data:     data,
		Metadata: meta,
	}

	if err := c.facade.FacadeCall("UploadDirect", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	return &result, nil
}
