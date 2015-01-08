// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type networkSuite struct {
	google.BaseSuite
}

var _ = gc.Suite(&networkSuite{})

func (s *networkSuite) TestNetworkSpecPath(c *gc.C) {
	//spec := google.NetworkSpec{
	//	Name: "spam",
	//}
	//path := spec.path()

	//c.Check(path, gc.Equals, "spam")
}

func (s *networkSuite) TestNetworkSpecNewInterface(c *gc.C) {
}

func (s *networkSuite) TestFirewallSpec(c *gc.C) {
}
