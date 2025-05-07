// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/tc"

	"github.com/juju/juju/environs"
)

type environNetworkSuite struct {
	baseEnvironSuite
}

var _ = tc.Suite(&environNetworkSuite{})

func (s *environNetworkSuite) TestSupportsSpaces(c *tc.C) {
	netEnv, ok := environs.SupportsNetworking(s.env)
	c.Assert(ok, tc.IsTrue)

	spaceSupport, err := netEnv.SupportsSpaces()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spaceSupport, tc.IsTrue)
}
