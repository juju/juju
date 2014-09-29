// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"io"
	"io/ioutil"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// Download implements the API method.
func (c *Client) Download(id string) (io.ReadCloser, error) {
	var result params.BackupsDownloadDirectResult
	args := params.BackupsDownloadArgs{ID: id}
	if err := c.facade.FacadeCall("DownloadDirect", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	archive := ioutil.NopCloser(bytes.NewBuffer(result.Data))
	return archive, nil
}
