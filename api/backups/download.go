// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/httprequest"

	"github.com/juju/juju/apiserver/params"
)

type downloadParams struct {
	httprequest.Route `httprequest:"GET /backups"`
	Body              params.BackupsDownloadArgs `httprequest:",body"`
}

// Download returns an io.ReadCloser for the given backup id.
func (c *Client) Download(id string) (io.ReadCloser, error) {
	// Send the request.
	var resp *http.Response
	err := c.client.Call(
		&downloadParams{
			Body: params.BackupsDownloadArgs{
				ID: id,
			},
		},
		&resp,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return resp.Body, nil
}
