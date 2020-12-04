// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/state"
)

type APIAddresserTests struct {
	ctrlSt                   *state.State
	state                    *state.State
	facade                   APIAddresserFacade
	waitForModelWatchersIdle func(c *gc.C)
}

func NewAPIAddresserTests(facade APIAddresserFacade, ctrlSt, st *state.State, waitForModelWatchersIdle func(c *gc.C)) *APIAddresserTests {
	return &APIAddresserTests{
		ctrlSt:                   ctrlSt,
		state:                    st,
		facade:                   facade,
		waitForModelWatchersIdle: waitForModelWatchersIdle,
	}
}

type APIAddresserFacade interface {
	APIAddresses() ([]string, error)
	APIHostPorts() ([]network.ProviderHostPorts, error)
	WatchAPIHostPorts() (watcher.NotifyWatcher, error)
}

func (s *APIAddresserTests) TestAPIAddresses(c *gc.C) {
	hostPorts := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "0.1.2.3"),
	}

	err := s.state.SetAPIHostPorts(hostPorts)
	c.Assert(err, jc.ErrorIsNil)

	addresses, err := s.facade.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.DeepEquals, []string{"0.1.2.3:1234"})
}

func (s *APIAddresserTests) TestAPIHostPorts(c *gc.C) {
	ipv6Addr := network.NewScopedSpaceAddress("2001:DB8::1", network.ScopeCloudLocal)

	setServerAddrs := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(999, "0.1.2.24"),
		network.NewSpaceHostPorts(1234, "example.com"),
		network.SpaceAddressesWithPort([]network.SpaceAddress{ipv6Addr}, 999),
	}
	err := s.state.SetAPIHostPorts(setServerAddrs)
	c.Assert(err, jc.ErrorIsNil)

	expectServerAddrs := []network.ProviderHostPorts{
		{network.ProviderHostPort{ProviderAddress: network.NewProviderAddress("0.1.2.24"), NetPort: 999}},
		{network.ProviderHostPort{ProviderAddress: network.NewProviderAddress("example.com"), NetPort: 1234}},
		{network.ProviderHostPort{ProviderAddress: network.NewProviderAddress(ipv6Addr.Value), NetPort: 999}},
	}
	expectServerAddrs[2][0].Scope = network.ScopeCloudLocal

	serverAddrs, err := s.facade.APIHostPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverAddrs, gc.DeepEquals, expectServerAddrs)
}

func (s *APIAddresserTests) TestWatchAPIHostPorts(c *gc.C) {
	hostports, err := s.ctrlSt.APIHostPortsForAgents()
	c.Assert(err, jc.ErrorIsNil)
	expectServerAddrs := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(5678, "0.1.2.3"),
	}
	// Make sure we are changing the value
	c.Assert(hostports, gc.Not(gc.DeepEquals), expectServerAddrs)
	s.waitForModelWatchersIdle(c)

	c.Logf("starting api host port watcher")
	w, err := s.facade.WatchAPIHostPorts()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w, s.state.StartSync)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertOneChange()
	c.Logf("got initial event")

	// Change the state addresses and check that we get a notification
	err = s.state.SetAPIHostPorts(expectServerAddrs)
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertOneChange()
	c.Logf("saw change event")

	// And that we can change it again and see the notification
	expectServerAddrs[0][0].Value = "0.1.99.99"

	err = s.state.SetAPIHostPorts(expectServerAddrs)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("saw second change event")

	wc.AssertOneChange()
}
