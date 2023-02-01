// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pebble_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/pebble"
)

type containerSuite struct{}

var _ = gc.Suite(&containerSuite{})

// TestClientForContainerCreation is a simple test to check that a Pebble client
// can still be created for a fictitious container. We have no reason to check
// the internal workings of the Pebble client creation process.
func (_ *containerSuite) TestClientForContainerCreation(c *gc.C) {
	_, err := pebble.ClientForContainer("testmctestface")	
	c.Assert(err, jc.ErrorIsNil)
}

func (_ *containerSuite) TestSocketPathForContainer(c *gc.C) {
	path := pebble.SocketPathForContainer("testmctestface")
	c.Assert(path, gc.Equals, "/charm/containers/testmctestface/pebble.socket")
}
