// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Result, gc.DeepEquals, []string{"addresses:1", "addresses:2"})
}

func (s *apiAddresserSuite) SetUpTest(c *gc.C) {
	s.addresser = common.NewAPIAddresser(fakeAddresses{}, common.NewResources())
}

func (s *apiAddresserSuite) TestAPIAddresses(c *gc.C) {
	result, err := s.addresser.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Result, gc.DeepEquals, []string{"apiaddresses:1", "apiaddresses:2"})
}

func (s *apiAddresserSuite) TestCACert(c *gc.C) {
	result := s.addresser.CACert()
	c.Assert(string(result.Result), gc.Equals, "a cert")
}

func (s *apiAddresserSuite) TestEnvironUUID(c *gc.C) {
	result := s.addresser.EnvironUUID()
	c.Assert(string(result.Result), gc.Equals, "the environ uuid")
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

func (fakeAddresses) EnvironUUID() string {
	return "the environ uuid"
}

func (fakeAddresses) APIHostPorts() ([][]network.HostPort, error) {
	return [][]network.HostPort{
		network.NewHostPorts(1, "apiaddresses"),
		network.NewHostPorts(2, "apiaddresses"),
	}, nil
}

func (fakeAddresses) WatchAPIHostPorts() state.NotifyWatcher {
	panic("should never be called")
}
