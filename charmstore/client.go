// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"io"

	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

// Client exposes the functionality of the charm store, as provided
// by github.com/juju/charmrepo/csclient.Client.
//
// Note that the following csclient.Client methods are used as well,
// but only in tests:
//  - Put(path string, val interface{}) error
//  - UploadCharm(id *charm.URL, ch charm.Charm) (*charm.URL, error)
//  - UploadCharmWithRevision(id *charm.URL, ch charm.Charm, promulgatedRevision int) error
//  - UploadBundleWithRevision()
type Client interface {
	// TODO(ericsnow) Replace use of Get with use of more specific API methods?

	// Get makes a GET request to the given path in the charm store. The
	// path must have a leading slash, but must not include the host
	// name or version prefix. The result is parsed as JSON into the
	// given result value, which should be a pointer to the expected
	// data, but may be nil if no result is desired.
	Get(path string, result interface{}) error

	// TODO(ericsnow) Just embed resource/charmstore.BaseClient?

	// ListResources composes, for each of the identified charms, the
	// list of details for each of the charm's resources. Those details
	// are those associated with the specific charm revision. They
	// include the resource's metadata and revision.
	ListResources(charmURLs []*charm.URL) ([][]charmresource.Resource, error)

	// GetResource returns a reader for the resource's data. That data
	// is streamed from the charm store. The charm's revision, if any,
	// is ignored. If the identified resource is not in the charm store
	// then errors.NotFound is returned.
	GetResource(cURL *charm.URL, resourceName string, revision int) (io.ReadCloser, error)
}
