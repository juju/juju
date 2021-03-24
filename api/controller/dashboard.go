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
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/params"
)

const (
	dashboardArchivePath = "/dashboard-archive"
	dashboardVersionPath = "/dashboard-version"
)

// DashboardArchives retrieves information about Juju Dashboard archives currently present
// in the Juju controller.
func (c *Client) DashboardArchives() ([]params.DashboardArchiveVersion, error) {
	httpClient, err := c.facade.RawAPICaller().HTTPClient()
	if err != nil {
		return nil, errors.Annotate(err, "cannot retrieve HTTP client")
	}
	var resp params.DashboardArchiveResponse
	if err = httpClient.Get(c.facade.RawAPICaller().Context(), dashboardArchivePath, &resp); err != nil {
		return nil, errors.Annotate(err, "cannot retrieve Dashboard archives info")
	}
	return resp.Versions, nil
}

// UploadDashboardArchive uploads a Dashboard archive to the controller over HTTPS, and
// reports about whether the upload updated the current Dashboard served by Juju.
func (c *Client) UploadDashboardArchive(r io.ReadSeeker, hash string, size int64, vers version.Number) (current bool, err error) {
	// Prepare the request.
	v := url.Values{}
	v.Set("version", vers.String())
	v.Set("hash", hash)
	req, err := http.NewRequest("POST", dashboardArchivePath+"?"+v.Encode(), r)
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
	var resp params.DashboardArchiveVersion
	if err = httpClient.Do(c.facade.RawAPICaller().Context(), req, &resp); err != nil {
		return false, errors.Annotate(err, "cannot upload the Dashboard archive")
	}
	return resp.Current, nil
}

// SelectDashboardVersion selects which version of the Juju Dashboard is served by the
// controller.
func (c *Client) SelectDashboardVersion(vers version.Number) error {
	// Prepare the request.
	content, err := json.Marshal(params.DashboardVersionRequest{
		Version: vers,
	})
	if err != nil {
		return errors.Annotate(err, "cannot marshal request body")
	}
	req, err := http.NewRequest("PUT", dashboardVersionPath, bytes.NewReader(content))
	if err != nil {
		return errors.Annotate(err, "cannot create PUT request")
	}
	req.Header.Set("Content-Type", params.ContentTypeJSON)

	// Retrieve a client and send the request.
	httpClient, err := c.facade.RawAPICaller().HTTPClient()
	if err != nil {
		return errors.Annotate(err, "cannot retrieve HTTP client")
	}
	if err = httpClient.Do(c.facade.RawAPICaller().Context(), req, nil); err != nil {
		return errors.Annotate(err, "cannot select Dashboard version")
	}
	return nil
}
