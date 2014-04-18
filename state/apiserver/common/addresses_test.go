// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
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

func (s *apiAddresserSuite) SetUpTest(c *gc.C) {
	s.addresser = common.NewAPIAddresser(fakeAddresses{}, common.NewResources())
}

func (s *apiAddresserSuite) TestAPIAddresses(c *gc.C) {
	result, err := s.addresser.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Result, gc.DeepEquals, []string{"apiaddresses:1", "apiaddresses:2"})
}

func (s *apiAddresserSuite) TestCACert(c *gc.C) {
	result := s.addresser.CACert()
	c.Assert(string(result.Result), gc.Equals, "a cert")
}

var _ common.AddressAndCertGetter = fakeAddresses{}

type fakeAddresses struct{}

func (fakeAddresses) Addresses() ([]string, error) {
	return []string{"addresses:1", "addresses:2"}, nil
}

func (fakeAddresses) APIAddressesFromMachines() ([]string, error) {
	panic("should never be called")
}

func (fakeAddresses) CACert() string {
	return "a cert"
}

func (fakeAddresses) APIHostPorts() ([][]instance.HostPort, error) {
	return [][]instance.HostPort{{{
		Address: instance.NewAddress("apiaddresses", instance.NetworkUnknown),
		Port:    1,
	}}, {{
		Address: instance.NewAddress("apiaddresses", instance.NetworkUnknown),
		Port:    2,
	}}}, nil
}

func (fakeAddresses) WatchAPIHostPorts() state.NotifyWatcher {
	panic("should never be called")
}
