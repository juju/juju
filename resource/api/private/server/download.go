// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"io"
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

// DownloadDataStore describes the the portion of Juju's "state"
// needed for handling download requests.
type DownloadDataStore interface {

	// OpenResource returns a reader that will stream the resource
	// content.
	OpenResource(name string) (resource.Resource, io.ReadCloser, error)
}

// HandleRequest handles a resource download request.
func handleDownload(store UnitDataStore, req *http.Request) (resource.Resource, io.ReadCloser, error) {
	name := api.ExtractDownloadRequest(req)
	res, resourceReader, err := store.OpenResource(name)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	return res, resourceReader, nil
}
