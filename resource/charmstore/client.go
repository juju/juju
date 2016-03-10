// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"io"

	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

// Client exposes the functionality of a charm store client as needed
// for charm store operations for Juju resources.
type Client interface {
	BaseClient
	io.Closer
}

// BaseClient exposes the functionality of charmrepo/csclient.Client,
// as used for charm store operations for Juju resources.
type BaseClient interface {
	// ListResources composes, for each of the identified charms, the
	// list of details for each of the charm's resources. Those details
	// are those associated with the specific charm revision. They
	// include the resource's metadata and revision.
	ListResources(charmURLs []*charm.URL) ([][]charmresource.Resource, error)

	// GetResource returns a reader for the resource's data. That data
	// is streamed from the charm store. The charm's revision, if any,
	// is ignored. If the identified resource is not in the charm store
	// then errors.NotFound is returned.
	GetResource(cURL *charm.URL, resourceName string, resourceRevision int) (io.ReadCloser, error)
}
