// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"gopkg.in/juju/charmrepo.v3/csclient"
	"gopkg.in/juju/charmrepo.v3/csclient/params"

	jjcharmstore "github.com/juju/juju/charmstore"
	"github.com/juju/juju/testcharms"
)

type charmstoreClientShim struct {
	*csclient.Client
}

func (c *charmstoreClientShim) WithChannel(channel params.Channel) CharmstoreForDeploy {
	return c.WithChannel(channel)
}

type fakeCharmstoreClientShim struct {
	*jjcharmstore.ChannelAwareFakeClient
}

func (c fakeCharmstoreClientShim) WithChannel(channel params.Channel) CharmstoreForDeploy {
	return c.WithChannel(channel)
}

// type charmstoreForDeployToTestcharmsCharmstoreShim struct {
// 	CharmstoreForDeploy
// }

// func (c charmstoreTestcharmsClientShim) WithChannel(channel params.Channel) testcharms.CharmstoreClient {
// 	return c.WithChannel(channel)
// }

type charmstoreTestcharmsClientShim struct {
	CharmstoreForDeploy
}

func (c charmstoreTestcharmsClientShim) WithChannel(channel params.Channel) testcharms.CharmstoreClient {
	return c.WithChannel(channel)
}

// type fakeCharmstoreClientShim struct {
// 	inner jjcharmstore.ChannelAwareFakeClient
// }

// func (c fakeCharmstoreClientShim) Get(path string, extra interface{}) error {
// 	return c.inner.Get(path, extra)
// }

// func (c fakeCharmstoreClientShim) Put(path string, extra interface{}) error {
// 	return c.inner.Put(path, extra)
// }

// func (c fakeCharmstoreClientShim) UploadBundle(id *charm.URL, bundle charm.Bundle) (*charm.URL, error) {
// 	return c.inner.UploadBundle(id, bundle)
// }

// func (c fakeCharmstoreClientShim) UploadBundleWithRevision(id *charm.URL, bundle charm.Bundle, promulgatedRevision int) error {
// 	return c.inner.UploadBundleWithRevision(id, bundle, promulgatedRevision)
// }

// func (c fakeCharmstoreClientShim) UploadCharm(id *charm.URL, charmDetails charm.Charm) (*charm.URL, error) {
// 	return c.inner.UploadCharm(id, charmDetails)
// }

// func (c fakeCharmstoreClientShim) UploadCharmWithRevision(id *charm.URL, charmDetails charm.Charm, promulgatedRevision int) error {
// 	return c.inner.UploadCharmWithRevision(id, charmDetails, promulgatedRevision)
// }

// func (c fakeCharmstoreClientShim) UploadResource(id *charm.URL, name, path string, file io.ReaderAt, size int64, progress csclient.Progress) (revision int, err error) {
// 	return c.inner.UploadResource(id, name, path, file, size, progress)
// }

// func (c fakeCharmstoreClientShim) AddDockerResource(id *charm.URL, resourceName string, imageName, digest string) (revision int, err error) {
// 	return c.inner.AddDockerResource(id, resourceName, imageName, digest)
// }

// func (c fakeCharmstoreClientShim) ListResources(id *charm.URL) ([]params.Resource, error) {
// 	return c.inner.ListResources(id)
// }

// func (c *fakeCharmstoreClientShim) WithChannel(channel csparams.Channel) CharmstoreForDeploy {
// 	return c.inner.WithChannel(channel)
// }
