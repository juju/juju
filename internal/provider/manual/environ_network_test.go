// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
)

type environNetworkSuite struct {
	baseEnvironSuite
}

var _ = gc.Suite(&environNetworkSuite{})

func (s *environNetworkSuite) TestSupportsSpaces(c *gc.C) {
	netEnv, ok := environs.SupportsNetworking(s.env)
	c.Assert(ok, jc.IsTrue)

	spaceSupport, err := netEnv.SupportsSpaces(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spaceSupport, jc.IsTrue)
}
