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

// Upload sends the backup archive to remote storage.
func (c *Client) Upload(archive io.Reader, meta params.BackupsMetadataResult) (_ *params.BackupsMetadataResult, err error) {
	logger.Debugf("preparing upload request")
	defer func() {
		if err != nil {
			logger.Debugf("upload request failed")
		}
	}()

	// Send the request.
	logger.Debugf("sending upload request")
	_, resp, err := c.http.SendHTTPRequestReader("PUT", "backups", archive, &meta, "juju-backup.tar.gz")
	if err != nil {
		return nil, errors.Annotate(err, "while sending HTTP request")
	}

	// Handle the response.
	if resp.StatusCode == http.StatusOK {
		var result params.BackupsMetadataResult
		if err := apihttp.ExtractJSONResult(resp, &result); err != nil {
			return nil, errors.Annotate(err, "while extracting result")
		}
		logger.Debugf("upload request succeeded (%s)", result.ID)
		return &result, nil
	} else {
		failure, err := apihttp.ExtractAPIError(resp)
		if err != nil {
			return nil, errors.Annotate(err, "while extracting failure")
		}
		return nil, errors.Trace(failure)
	}
}
