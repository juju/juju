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

// HandleDownload handles a resource download request.
func HandleDownload(req *http.Request, deps HandleDownloadDeps) (resource.Resource, io.ReadCloser, error) {
	name := deps.ExtractDownloadRequest(req)
	res, resourceReader, err := deps.OpenResource(name)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	return res, resourceReader, nil
}

// HandledDownloadDeps exposes the external dependencies of HandleDownload.
type HandleDownloadDeps interface {
	DownloadDataStore

	// ExtractDownloadRequest pulls the resource name from the request.
	ExtractDownloadRequest(*http.Request) string
}

type handleDownloadDeps struct {
	DownloadDataStore
}

func (handleDownloadDeps) ExtractDownloadRequest(req *http.Request) string {
	return api.ExtractDownloadRequest(req)
}
