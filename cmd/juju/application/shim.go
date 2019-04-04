// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"gopkg.in/juju/charmrepo.v3/csclient"
	"gopkg.in/juju/charmrepo.v3/csclient/params"

	"github.com/juju/juju/testcharms"
)

type charmstoreClientShim struct {
	*csclient.Client
}

func (c charmstoreClientShim) WithChannel(channel params.Channel) charmstoreForDeploy {
	client := c.Client.WithChannel(channel)
	return charmstoreClientShim{client}
}

type charmstoreClientToTestcharmsClientShim struct {
	*csclient.Client
}

func (c charmstoreClientToTestcharmsClientShim) WithChannel(channel params.Channel) testcharms.CharmstoreClient {
	client := c.Client.WithChannel(channel)
	return charmstoreClientToTestcharmsClientShim{client}
}
