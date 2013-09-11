// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	gc "launchpad.net/gocheck"
)

type stateSuite struct {
	uniterSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)
}

func (s *stateSuite) TearDownTest(c *gc.C) {
	s.uniterSuite.TearDownTest(c)
}

func (s *stateSuite) TestAPIAddresses(c *gc.C) {
	apiInfo := s.APIInfo(c)

	addresses, err := s.uniter.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addresses, gc.DeepEquals, apiInfo.Addrs)
}
