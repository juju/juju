// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/apiserver/common"
)

type stateAddresserSuite struct {
	addresser *common.StateAddresser
}

type apiAddresserSuite struct {
	addresser *common.APIAddresser
}

var _ = gc.Suite(&stateAddresserSuite{})
var _ = gc.Suite(&apiAddresserSuite{})

func (s *stateAddresserSuite) SetUpTest(c *gc.C) {
	s.addresser = common.NewStateAddresser(fakeAddresses{})
}

// Verify that AddressAndCertGetter is satisfied by *state.State.
var _ common.AddressAndCertGetter = (*state.State)(nil)

func (s *stateAddresserSuite) TestStateAddresses(c *gc.C) {
	result, err := s.addresser.StateAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Result, gc.DeepEquals, []string{"addresses:1", "addresses:2"})
}

func (s *stateAddresserSuite) TestCACert(c *gc.C) {
	result := s.addresser.CACert()
	c.Assert(string(result.Result), gc.Equals, "a cert")
}

func (s *apiAddresserSuite) SetUpTest(c *gc.C) {
	s.addresser = common.NewAPIAddresser(fakeAddresses{})
}

func (s *apiAddresserSuite) TestAPIAddresses(c *gc.C) {
	result, err := s.addresser.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Result, gc.DeepEquals, []string{"apiaddresses:1", "apiaddresses:2"})
}

type fakeAddresses struct{}

func (fakeAddresses) Addresses() ([]string, error) {
	return []string{"addresses:1", "addresses:2"}, nil
}

func (fakeAddresses) APIAddresses() ([]string, error) {
	return []string{"apiaddresses:1", "apiaddresses:2"}, nil
}

func (fakeAddresses) CACert() []byte {
	return []byte("a cert")
}
