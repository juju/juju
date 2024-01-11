// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"
	"io"
	"net/http"

	"github.com/juju/errors"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/rpc/params"
)

type downloadParams struct {
	httprequest.Route `httprequest:"GET /backups"`
	Body              params.BackupsDownloadArgs `httprequest:",body"`
}

// Download returns an io.ReadCloser for the given backup id.
func (c *Client) Download(ctx context.Context, filename string) (io.ReadCloser, error) {
	httpClient, err := c.st.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var resp *http.Response
	err = httpClient.Call(
		ctx,
		&downloadParams{
			Body: params.BackupsDownloadArgs{
				ID: filename,
			},
		},
		&resp,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return resp.Body, nil
}
