// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
)

var logger = loggo.GetLogger("juju.charmstore")

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
	BaseClient
	// TODO(ericsnow) Move this over to BaseClient once it supports it.
	ResourcesClient
	io.Closer

	// AsRepo returns a charm repo that wraps the client. If the model's
	// UUID is provided then it is associated with all of the repo's
	// interaction with the charm store.
	AsRepo(modelUUID string) Repo
}

// BaseClient exposes the functionality Juju needs which csclient.Client
// provides.
type BaseClient interface {
	// TODO(ericsnow) Replace use of Get with use of more specific API
	// methods? We only use Get() for authorization on the Juju client
	// side.

	// Get makes a GET request to the given path in the charm store. The
	// path must have a leading slash, but must not include the host
	// name or version prefix. The result is parsed as JSON into the
	// given result value, which should be a pointer to the expected
	// data, but may be nil if no result is desired.
	Get(path string, result interface{}) error
}

// ResourcesClient exposes the charm store client functionality for
// dealing with resources.
type ResourcesClient interface {
	// TODO(ericsnow) Just embed resource/charmstore.BaseClient (or vice-versa)?

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

// client adapts csclient.Client to the needs of Juju.
type client struct {
	*csclient.Client
	fakeCharmStoreClient
	io.Closer
}

// NewClient returns a Juju charm store client that wraps the provided
// client.
func NewClient(base *csclient.Client, closer io.Closer) Client {
	return &client{
		Client: base,
		Closer: closer,
	}
}

// AsRepo implements Client.
func (client client) AsRepo(envUUID string) Repo {
	repo := charmrepo.NewCharmStoreFromClient(client.Client)
	if envUUID != "" {
		repo = repo.WithJujuAttrs(map[string]string{
			"environment_uuid": envUUID,
		})
	}
	return repo
}

// Close implements Client.
func (client client) Close() error {
	if client.Closer == nil {
		return nil
	}
	return client.Closer.Close()
}

// LatestCharmInfo returns the most up-to-date information about each
// of the identified charms at their latest revision. The revisions in
// the provided URLs are ignored.
func LatestCharmInfo(client Client, modelUUID string, cURLs []*charm.URL) ([]CharmInfoResult, error) {
	// We must use charmrepo.CharmStore since csclient.Client does not
	// have the "Latest" method.
	repo := client.AsRepo(client.modelUUID)

	// Do a bulk call to get the revision info for all charms.
	logger.Infof("retrieving revision information for %d charms", len(refs))
	revResults, err := repo.Latest(refs...)
	if err != nil {
		err = errors.Annotate(err, "finding charm revision info")
		logger.Infof(err.Error())
		return nil, errors.Trace(err)
	}

	// Extract the results.
	var results []CharmInfoResult
	for i, ref := range refs {
		revResult := revResults[i]

		var result CharmInfoResult
		if revResult.Err != nil {
			result.Error = errors.Trace(revResult.Err)
		} else {
			result.URL = ref.WithRevision(revResult.Revision)
		}
		results = append(results, result)
	}
	return results, nil
}

// TODO(ericsnow) Remove the fake once the charm store adds support.

type fakeCharmStoreClient struct{}

// ListResources implements Client as a noop.
func (fakeCharmStoreClient) ListResources(charmURLs []*charm.URL) ([][]charmresource.Resource, error) {
	res := make([][]charmresource.Resource, len(charmURLs))
	return res, nil
}

// GetResource implements Client as a noop.
func (fakeCharmStoreClient) GetResource(cURL *charm.URL, resourceName string, revision int) (io.ReadCloser, error) {
	return nil, errors.NotFoundf("resource %q", resourceName)
}
