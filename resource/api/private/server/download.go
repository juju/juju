// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"io"
	"net/http"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

// DownloadDataStore describes the the portion of Juju's "state"
// needed for handling download requests.
type DownloadDataStore interface {

	// GetResource returns the resource data for the identified resource.
	GetResource(name string) (resource.Resource, error)

	// OpenResource returns a reader that will stream the resource
	// content.
	OpenResource(name string) (resource.Resource, io.ReadCloser, error)
}

// HandleDownload handles a resource download request.
func HandleDownload(name string, deps HandleDownloadDeps) (resource.Resource, io.ReadCloser, error) {
	res, resourceReader, err := deps.OpenResource(name)
	if errors.IsNotFound(err) {
		res, resourceReader, err = openFromCharmstore(name, deps, err)
	}
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	return res, resourceReader, nil
}

func openFromCharmstore(name string, deps HandleDownloadDeps, openErr error) (resource.Resource, io.ReadCloser, error) {
	res, err := deps.GetResource(name)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}
	if res.Origin != charmresource.OriginStore {
		return resource.Resource{}, nil, errors.Trace(openErr)
	}

	csClient, err := deps.NewCharmstoreClient()
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}
	defer csClient.Close()

	resourceReader, err := csClient.GetResource(name)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	return res, resourceReader, nil
}

// CharmstoreClient exposes the charm store functionality needed
// for the hook context.
type CharmstoreClient interface {
	io.Closer

	// GetResource returns a reader for the resource data. The resource
	// will also be appropriately stored in state.
	GetResource(name string) (io.ReadCloser, error)
}

// CharmstoreOpener exposes the functionality to open a connection
// to the charm store.
type CharmstoreOpener interface {
	// NewCharmstoreClient returns a new charm store client to use
	// in HandleDownload().
	NewCharmstoreClient() (CharmstoreClient, error)
}

// HandledDownloadDeps exposes the external dependencies of HandleDownload.
type HandleDownloadDeps interface {
	DownloadDataStore
	CharmstoreOpener

	// ExtractDownloadRequest pulls the resource name from the request.
	ExtractDownloadRequest(*http.Request) string
}

type handleDownloadDeps struct {
	DownloadDataStore
	CharmstoreOpener
}

func (handleDownloadDeps) ExtractDownloadRequest(req *http.Request) string {
	return api.ExtractDownloadRequest(req)
}
