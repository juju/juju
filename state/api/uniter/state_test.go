// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	apitesting "launchpad.net/juju-core/state/api/common/testing"
	
	gc "launchpad.net/gocheck"
)

type stateSuite struct {
	uniterSuite
	*apitesting.APIAddresserTests
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)
	s.APIAddresserTests = apitesting.NewAPIAddresserTests(s.State, s.uniter)
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
