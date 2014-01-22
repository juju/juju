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

func (s *stateSuite) TestAPIAddresses(c *gc.C) {
	stateAPIAddresses, err := s.State.APIAddresses()
	c.Assert(err, gc.IsNil)
	addresses, err := s.uniter.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addresses, gc.DeepEquals, stateAPIAddresses)
	// testing.AddStateServerMachine creates a machine which does *not*
	// match the values in the Environ Config, so these don't match
	apiInfo := s.APIInfo(c)
	c.Assert(addresses, gc.Not(gc.DeepEquals), apiInfo.Addrs)
}

func (s *stateSuite) TestProviderType(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)

	providerType, err := s.uniter.ProviderType()
	c.Assert(err, gc.IsNil)
	c.Assert(providerType, gc.DeepEquals, cfg.Type())
}

type noStateServerSuite struct {
	uniterSuite
}

var _ = gc.Suite(&noStateServerSuite{})

func (s *noStateServerSuite) SetUpTest(c *gc.C) {
	// avoid adding the state server machine.
	s.setUpTest(c, false)
}

func (s *noStateServerSuite) TestAPIAddressesFailure(c *gc.C) {
	_, err := s.uniter.APIAddresses()
	c.Assert(err, gc.ErrorMatches, "no state server machines found")
}
