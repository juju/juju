// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/local"
)

type environSuite struct {
	providerSuite
}

var _ = Suite(&environSuite{})

func (*environSuite) TestName(c *C) {
	testConfig := minimalConfig(c)

	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, IsNil)

	c.Assert(environ.Name(), Equals, "test")
}
