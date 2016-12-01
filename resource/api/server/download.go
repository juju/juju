// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"io"
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/resource"
)

// XXX tests

// DownloadDataStore describes the the portion of Juju's "state"
// needed for handling resource download requests.
type DownloadDataStore interface {
	// OpenResource returns the identified resource and its content.
	OpenResource(applicationID, name string) (resource.Resource, io.ReadCloser, error)
}

// DownloadHandler provides the functionality to handle download requests.
type DownloadHandler struct {
	// Store is the data store from the resource will be retrieved.
	Store DownloadDataStore
}

// HandleRequest handles a resource download request.
func (dh DownloadHandler) HandleRequest(req *http.Request) (io.ReadCloser, int64, error) {
	defer req.Body.Close()

	query := req.URL.Query()
	application := query.Get(":application")
	name := query.Get(":resource")

	resource, reader, err := dh.Store.OpenResource(application, name)
	return reader, resource.Size, errors.Trace(err)
}
