// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/apiserver/params"
)

const (
	guiArchivePath = "/gui-archive"
	guiVersionPath = "/gui-version"
)

// GUIArchives retrieves information about Juju GUI archives currently present
// in the Juju controller.
func (c *Client) GUIArchives() ([]params.GUIArchiveVersion, error) {
	httpClient, err := c.facade.RawAPICaller().HTTPClient()
	if err != nil {
		return nil, errors.Annotate(err, "cannot retrieve HTTP client")
	}
	var resp params.GUIArchiveResponse
	if err = httpClient.Get(guiArchivePath, &resp); err != nil {
		return nil, errors.Annotate(err, "cannot retrieve GUI archives info")
	}
	return resp.Versions, nil
}

// UploadGUIArchive uploads a GUI archive to the controller over HTTPS, and
// reports about whether the upload updated the current GUI served by Juju.
func (c *Client) UploadGUIArchive(r io.ReadSeeker, hash string, size int64, vers version.Number) (current bool, err error) {
	// Prepare the request.
	v := url.Values{}
	v.Set("version", vers.String())
	v.Set("hash", hash)
	req, err := http.NewRequest("POST", guiArchivePath+"?"+v.Encode(), nil)
	if err != nil {
		return false, errors.Annotate(err, "cannot create upload request")
	}
	req.Header.Set("Content-Type", "application/x-tar-bzip2")
	req.ContentLength = size

	// Retrieve a client and send the request.
	httpClient, err := c.facade.RawAPICaller().HTTPClient()
	if err != nil {
		return false, errors.Annotate(err, "cannot retrieve HTTP client")
	}
	var resp params.GUIArchiveVersion
	if err = httpClient.Do(req, r, &resp); err != nil {
		return false, errors.Annotate(err, "cannot upload the GUI archive")
	}
	return resp.Current, nil
}

// SelectGUIVersion selects which version of the Juju GUI is served by the
// controller.
func (c *Client) SelectGUIVersion(vers version.Number) error {
	// Prepare the request.
	req, err := http.NewRequest("PUT", guiVersionPath, nil)
	if err != nil {
		return errors.Annotate(err, "cannot create PUT request")
	}
	req.Header.Set("Content-Type", params.ContentTypeJSON)
	content, err := json.Marshal(params.GUIVersionRequest{
		Version: vers,
	})
	if err != nil {
		errors.Annotate(err, "cannot marshal request body")
	}

	// Retrieve a client and send the request.
	httpClient, err := c.facade.RawAPICaller().HTTPClient()
	if err != nil {
		return errors.Annotate(err, "cannot retrieve HTTP client")
	}
	if err = httpClient.Do(req, bytes.NewReader(content), nil); err != nil {
		return errors.Annotate(err, "cannot select GUI version")
	}
	return nil
}
