// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"gopkg.in/juju/charmrepo.v3/csclient"
	"gopkg.in/juju/charmrepo.v3/csclient/params"

	"github.com/juju/juju/testcharms"
)

type testcharmsClientShim struct {
	*csclient.Client
}

func (c *testcharmsClientShim) WithChannel(channel params.Channel) testcharms.CharmstoreClient {
	return c.WithChannel(channel)
}
