// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"

	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs"
	jujulxdclient "github.com/juju/juju/tools/lxdclient"
)

// TODO (manadart 2018-05-09) This is really nothing but an LXD server and does
// not need its own type.
//
// Side-note on terms: what used to be called a client will be our new "server".
// This is for congruence with the LXD package, which presents things like
// "ContainerServer" and "ImageServer" for interaction with LXD.
//
// As the LXD facility is refactored, this will be removed altogether.
// As an interim measure, the new and old client implementations will be have
// interface shims.
// After the old client is removed, provider tests can be rewritten using
// GoMock, at which point rawProvider is replaced with the new server.

func newServer(spec environs.CloudSpec, local bool) (Server, error) {
	if local {
		prov, err := newLocalProvider()
		return prov, errors.Trace(err)
	}
	clientCert, serverCert, ok := getCertificates(spec)
	if !ok {
		return nil, errors.NotValidf("credentials")
	}
	serverSpec := lxd.NewServerSpec(spec.Endpoint, serverCert, clientCert)
	prov, err := lxd.NewRemoteServer(serverSpec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return prov, nil
}

func newLocalProvider() (Server, error) {
	config := jujulxdclient.Config{Remote: jujulxdclient.Local}
	raw, err := newProviderFromConfig(config)
	return raw, errors.Trace(err)
}

func newProviderFromConfig(config jujulxdclient.Config) (Server, error) {
	client, err := jujulxdclient.Connect(config, true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client, nil
}
