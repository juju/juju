// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Download implements the API method.
func (c *Client) Download(id string) (io.ReadCloser, error) {
	if c.http == nil {
		msg := "API client does not support direct HTTP requests"
		return nil, errors.NotImplementedf(msg)
	}

	// Initialize the HTTP request.
	req, err := c.http.NewHTTPRequest("GET", "backups")
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Serialize the args.
	req.Header.Set("Content-Type", "application/json")
	args := params.BackupsDownloadArgs{
		ID: id,
	}
	data, err := json.Marshal(&args)
	if err != nil {
		return nil, errors.Annotate(err, "while serializing args")
	}
	req.Body = ioutil.NopCloser(bytes.NewBuffer(data))

	// Send the request.
	resp, err := c.http.SendHTTPRequest(req)
	if err != nil {
		return nil, errors.Annotate(err, "while sending HTTP request")
	}

	// Handle the response.
	if resp.StatusCode != http.StatusOK {
		failure, err := base.HandleHTTPFailure(resp)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return nil, errors.Trace(failure)
	}

	return resp.Body, nil
}
