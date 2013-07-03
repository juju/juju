// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/local"
)

type environSuite struct {
	baseProviderSuite
}

var _ = gc.Suite(&environSuite{})

func (*environSuite) TestName(c *gc.C) {
	testConfig := minimalConfig(c)

	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, gc.IsNil)

	c.Assert(environ.Name(), gc.Equals, "test")
}
