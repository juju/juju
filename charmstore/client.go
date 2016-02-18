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
	ListResources(charmURLs []charm.URL) ([][]charmresource.Resource, error)

	// GetResource returns a reader for the resource's data. That data
	// is streamed from the charm store. The charm's revision, if any,
	// is ignored. If the identified resource is not in the charm store
	// then errors.NotFound is returned.
	GetResource(cURL *charm.URL, resourceName string, revision int) (io.ReadCloser, error)
}

// TestingClient expands Client with methods needed during testing.
type TestingClient interface {
	Client

	// Put makes a PUT request to the given path in the charm store. The
	// path must have a leading slash, but must not include the host
	// name or version prefix. The given value is marshaled as JSON to
	// use as the request body.
	Put(path string, val interface{}) error

	// UploadCharm uploads the given charm to the charm store with the
	// given id, which must not specify a revision. The accepted charm
	// implementations are charm.CharmDir and charm.CharmArchive.
	//
	// UploadCharm returns the id that the charm has been given in the
	// store - this will be the same as id except the revision.
	UploadCharm(id *charm.URL, ch charm.Charm) (*charm.URL, error)

	// UploadCharmWithRevision uploads the given charm to the given id
	// in the charm store, which must contain a revision. If
	// promulgatedRevision is not -1, it specifies that the charm should
	// be marked as promulgated with that revision.
	UploadCharmWithRevision(id *charm.URL, ch charm.Charm, promulgatedRevision int) error

	// UploadBundleWithRevision uploads the given bundle to the given id
	// in the charm store, which must contain a revision. If
	// promulgatedRevision is not -1, it specifies that the charm should
	// be marked as promulgated with that revision.
	UploadBundleWithRevision()
}
