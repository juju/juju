// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/juju/v2/environs"
)

// Filtering exports
var (
	MatchPortRanges = matchPortRanges
	MatchSubnet     = matchSubnet
)

func SetNewEnviron(c *Client, newEnviron func() (environs.BootstrapEnviron, error)) {
	c.newEnviron = newEnviron
}

// OverrideClientBackendMongoSession is necessary to provide the tests a way to
// return a mongo session that will pretend to have a happy replicaset.
// This is necessary right now as the tests don't run with a replicaset for
// speed.
func OverrideClientBackendMongoSession(c *Client, session MongoSession) {
	c.api.stateAccessor.(*stateShim).session = session
}
