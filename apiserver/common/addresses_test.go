// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type apiAddresserSuite struct {
	addresser *common.APIAddresser
	fake      *fakeAddresses
}

var _ = gc.Suite(&apiAddresserSuite{})

// Verify that APIAddressAccessor is satisfied by *state.State.
var _ common.APIAddressAccessor = (*state.State)(nil)

func (s *apiAddresserSuite) SetUpTest(c *gc.C) {
	s.fake = &fakeAddresses{
		hostPorts: []network.SpaceHostPorts{
			network.NewSpaceHostPorts(1, "apiaddresses"),
			network.NewSpaceHostPorts(2, "apiaddresses"),
		},
	}
	s.addresser = common.NewAPIAddresser(s.fake, common.NewResources())
}

func (s *apiAddresserSuite) TestAPIAddresses(c *gc.C) {
	result, err := s.addresser.APIAddresses(coretesting.FakeControllerConfig())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Result, gc.DeepEquals, []string{"apiaddresses:1", "apiaddresses:2"})
}

func (s *apiAddresserSuite) TestAPIAddressesPrivateFirst(c *gc.C) {
	addrs := network.NewSpaceAddresses("52.7.1.1", "10.0.2.1")
	ctlr1 := network.SpaceAddressesWithPort(addrs, 17070)

	addrs = network.NewSpaceAddresses("53.51.121.17", "10.0.1.17")
	ctlr2 := network.SpaceAddressesWithPort(addrs, 17070)

	s.fake.hostPorts = []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1, "apiaddresses"),
		ctlr1,
		ctlr2,
		network.NewSpaceHostPorts(2, "apiaddresses"),
	}
	for _, hps := range s.fake.hostPorts {
		for _, hp := range hps {
			c.Logf("%s - %#v", hp.Scope, hp)
		}
	}

	result, err := s.addresser.APIAddresses(coretesting.FakeControllerConfig())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result.Result, gc.DeepEquals, []string{
		"apiaddresses:1",
		"10.0.2.1:17070",
		"52.7.1.1:17070",
		"10.0.1.17:17070",
		"53.51.121.17:17070",
		"apiaddresses:2",
	})
}

var _ common.APIAddressAccessor = fakeAddresses{}

type fakeAddresses struct {
	hostPorts []network.SpaceHostPorts
}

func (fakeAddresses) ControllerConfig() (controller.Config, error) {
	return coretesting.FakeControllerConfig(), nil
}

func (f fakeAddresses) APIHostPortsForAgents(controller.Config) ([]network.SpaceHostPorts, error) {
	return f.hostPorts, nil
}

func (fakeAddresses) WatchAPIHostPortsForAgents() state.NotifyWatcher {
	panic("should never be called")
}
