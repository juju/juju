// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"net/http"

	"github.com/juju/errors"

	apihttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
)

// Download returns an io.ReadCloser for the given backup id.
func (c *Client) Download(id string) (io.ReadCloser, error) {
	// Send the request.
	args := params.BackupsDownloadArgs{
		ID: id,
	}
	_, resp, err := c.http.SendHTTPRequest("backups", &args)
	if err != nil {
		return nil, errors.Annotate(err, "while sending HTTP request")
	}

	// Handle the response.
	if resp.StatusCode != http.StatusOK {
		failure, err := apihttp.ExtractAPIError(resp)
		if err != nil {
			return nil, errors.Annotate(err, "while extracting failure")
		}
		return nil, errors.Trace(failure)
	}

	return resp.Body, nil
}
