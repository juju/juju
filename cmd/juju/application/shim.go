// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"gopkg.in/juju/charmrepo.v3/csclient"
	"gopkg.in/juju/charmrepo.v3/csclient/params"
	csparams "gopkg.in/juju/charmrepo.v3/csclient/params"

	jjcharmstore "github.com/juju/juju/charmstore"
	"github.com/juju/juju/testcharms"
)

type charmstoreClientShim struct {
	*csclient.Client
}

func (c *charmstoreClientShim) WithChannel(channel params.Channel) CharmstoreForDeploy {
	return c.WithChannel(channel)
}

type charmstoreTestcharmsClientShim struct {
	CharmstoreForDeploy
}

func (c *charmstoreTestcharmsClientShim) WithChannel(channel params.Channel) testcharms.CharmstoreClient {
	return c.WithChannel(channel)
}

type fakeCharmstoreClientShim struct {
	jjcharmstore.ChannelAwareFakeClient
}

// func (c fakeCharmstoreClientShim) Get(path string, extra interface{}) error {
// 	return c.internal.Get(path, extra)
// }

// func (c fakeCharmstoreClientShim) Put(path string, extra interface{}) error {
// 	return c.internal.Put(path, extra)
// }

// func (c fakeCharmstoreClientShim) AddDockerResource(id *charm.URL, resourceName string, imageName, digest string) (revision int, err error) {
// 	return c.internal.AddDockerResource(id, resourceName, imageName, digest)
// }

// func (c fakeCharmstoreClientShim) ListResources(id *charm.URL) ([]params.Resource, error) {
// 	return c.internal.ListResources(id)
// }

func (c *fakeCharmstoreClientShim) WithChannel(channel csparams.Channel) CharmstoreForDeploy {
	return c.WithChannel(channel)
}
