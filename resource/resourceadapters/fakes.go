// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"io"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

// TODO(ericsnow) Get rid of fakeCharmStoreClient once csclient.Client grows the methods.

type fakeCharmStoreClient struct{}

// ListResources implements resource/charmstore.Client as a noop.
func (fakeCharmStoreClient) ListResources(charmURLs []charm.URL) ([][]charmresource.Resource, error) {
	res := make([][]charmresource.Resource, len(charmURLs))
	return res, nil
}

// GetResource implements resource/charmstore.Client as a noop.
func (fakeCharmStoreClient) GetResource(cURL *charm.URL, resourceName string, revision int) (io.ReadCloser, error) {
	return nil, errors.NotFoundf("resource %q", resourceName)
}

// Close implements io.Closer.
func (fakeCharmStoreClient) Close() error {
	return nil
}
