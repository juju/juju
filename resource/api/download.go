// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"net/http"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

// NewHTTPDownloadRequest creates a new HTTP download request
// for the given resource.
//
// Intended for use on the client side.
func NewHTTPDownloadRequest(resourceName string) (*http.Request, error) {
	return http.NewRequest("GET", "/resources/"+resourceName, nil)
}

// ExtractDownloadRequest pulls the download request info out of the
// given HTTP request.
//
// Intended for use on the server side.
func ExtractDownloadRequest(req *http.Request) string {
	return req.URL.Query().Get(":resource")
}

// UpdateDownloadResponse sets the appropriate headers in the response
// to an HTTP download request.
//
// Intended for use on the server side.
func UpdateDownloadResponse(resp http.ResponseWriter, resource resource.Resource) {
	resp.Header().Set("Content-Type", ContentTypeRaw)
	resp.Header().Set("Content-Length", fmt.Sprint(resource.Size))
	resp.Header().Set("Content-Sha384", resource.Fingerprint.String())
}

// ExtractDownloadResponse pulls the download size and checksum
// from the HTTP response.
func ExtractDownloadResponse(resp *http.Response) (int64, charmresource.Fingerprint, error) {
	var fp charmresource.Fingerprint

	// TODO(ericsnow) Finish!
	// See UpdateDownloadResponse for the data to extract.
	return 0, fp, errors.New("not finished")
}
