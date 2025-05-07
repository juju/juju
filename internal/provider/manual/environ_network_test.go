// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/environs"
)

type environNetworkSuite struct {
	baseEnvironSuite
}

var _ = tc.Suite(&environNetworkSuite{})

func (s *environNetworkSuite) TestSupportsSpaces(c *tc.C) {
	netEnv, ok := environs.SupportsNetworking(s.env)
	c.Assert(ok, jc.IsTrue)

	spaceSupport, err := netEnv.SupportsSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spaceSupport, jc.IsTrue)
}
