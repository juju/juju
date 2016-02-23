// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"io"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/cmd/juju/charmcmd"
	"github.com/juju/juju/resource/charmstore"
	"github.com/juju/juju/resource/cmd"
)

// TODO(ericsnow) Get rid of fakeCharmStoreClient once csclient.Client grows the methods.

type baseCharmStoreClient interface {
	io.Closer
}

func newFakeCharmStoreClient(base baseCharmStoreClient) charmstore.Client {
	return &fakeCharmStoreClient{base}
}

type fakeCharmStoreClient struct {
	baseCharmStoreClient
}

// ListResources implements resource/charmstore.Client as a noop.
func (fakeCharmStoreClient) ListResources(charmURLs []*charm.URL) ([][]charmresource.Resource, error) {
	res := make([][]charmresource.Resource, len(charmURLs))
	return res, nil
}

// GetResource implements resource/charmstore.Client as a noop.
func (fakeCharmStoreClient) GetResource(cURL *charm.URL, resourceName string, revision int) (io.ReadCloser, error) {
	return nil, errors.NotFoundf("resource %q", resourceName)
}

// Close implements io.Closer.
func (client fakeCharmStoreClient) Close() error {
	if client.baseCharmStoreClient == nil {
		return nil
	}
	return client.baseCharmStoreClient.Close()
}

// TODO(ericsnow) Get rid of fakeCharmCmdBase once csclient.Client grows the methods.

type fakeCharmCmdBase struct {
	*charmcmd.CommandBase
}

func NewFakeCharmCmdBase(base *charmcmd.CommandBase) cmd.CharmCommandBase {
	return &fakeCharmCmdBase{base}
}

// Connect implements cmd.CommandBase.
func (c *fakeCharmCmdBase) Connect() (cmd.CharmResourceLister, error) {
	client, err := c.CommandBase.Connect()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newFakeCharmStoreClient(client), nil
}
