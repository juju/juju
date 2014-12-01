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
func (c *Client) Download(id string) (_ io.ReadCloser, err error) {
	logger.Debugf("sending download request (%s)", id)
	defer func() {
		if err != nil {
			logger.Debugf("download request failed (%s)", id)
		}
	}()

	// Send the request.
	logger.Debugf("sending download request (%s)", id)
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
	logger.Debugf("download request succeeded (%s)", id)

	return resp.Body, nil
}
