// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"io"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/version"
)

// UploadGUIArchive uploads a GUI archive to the controller over HTTPS.
func (c *Client) UploadGUIArchive(r io.ReadSeeker, hash string, size int64, vers version.Number) error {
	// Prepare the request.
	v := url.Values{}
	v.Set("version", vers.String())
	v.Set("hash", hash)
	req, err := http.NewRequest("POST", "/gui-archive?"+v.Encode(), nil)
	if err != nil {
		return errors.Annotate(err, "cannot create upload request")
	}
	req.Header.Set("Content-Type", "application/x-tar-bzip2")
	req.ContentLength = size

	// Retrieve a client and send the request.
	httpClient, err := c.st.RootHTTPClient()
	if err != nil {
		return errors.Annotate(err, "cannot retrieve HTTP client")
	}
	if err = httpClient.Do(req, r, nil); err != nil {
		return errors.Annotate(err, "cannot upload the GUI archive")
	}
	return nil
}
