// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/watcher"
	statetesting "launchpad.net/juju-core/state/testing"
)

type APIAddresserTests struct {
	state  *state.State
	facade APIAddresserFacade
}

func NewAPIAddresserTests(facade APIAddresserFacade, st *state.State) *APIAddresserTests {
	return &APIAddresserTests{
		state:  st,
		facade: facade,
	}
}

type APIAddresserFacade interface {
	APIAddresses() ([]string, error)
	CACert() (string, error)
	APIHostPorts() ([][]instance.HostPort, error)
	WatchAPIHostPorts() (watcher.NotifyWatcher, error)
}

func (s *APIAddresserTests) TestAPIAddresses(c *gc.C) {
	hostPorts := [][]instance.HostPort{{{
		Address: instance.NewAddress("0.1.2.3", instance.NetworkUnknown),
		Port:    1234,
	}}}

	err := s.state.SetAPIHostPorts(hostPorts)
	c.Assert(err, gc.IsNil)

	addresses, err := s.facade.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addresses, gc.DeepEquals, []string{"0.1.2.3:1234"})
}

func (s *APIAddresserTests) TestAPIHostPorts(c *gc.C) {
	expectServerAddrs := [][]instance.HostPort{{{
		Address: instance.NewAddress("0.1.2.24", instance.NetworkUnknown),
		Port:    999,
	}, {
		Address: instance.NewAddress("example.com", instance.NetworkUnknown),
		Port:    1234,
	}}, {{
		Address: instance.Address{
			Value:        "2001:DB8::1",
			Type:         instance.Ipv6Address,
			NetworkName:  "someNetwork",
			NetworkScope: instance.NetworkCloudLocal,
		},
		Port: 999,
	}}}

	err := s.state.SetAPIHostPorts(expectServerAddrs)
	c.Assert(err, gc.IsNil)

	serverAddrs, err := s.facade.APIHostPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(serverAddrs, gc.DeepEquals, expectServerAddrs)
}

func (s *APIAddresserTests) TestCACert(c *gc.C) {
	caCert, err := s.facade.CACert()
	c.Assert(err, gc.IsNil)
	c.Assert(caCert, gc.DeepEquals, s.state.CACert())
}

func (s *APIAddresserTests) TestWatchAPIHostPorts(c *gc.C) {
	expectServerAddrs := [][]instance.HostPort{{{
		Address: instance.NewAddress("0.1.2.3", instance.NetworkUnknown),
		Port:    1234,
	}}}
	err := s.state.SetAPIHostPorts(expectServerAddrs)
	c.Assert(err, gc.IsNil)

	w, err := s.facade.WatchAPIHostPorts()
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)

	wc := statetesting.NewNotifyWatcherC(c, s.state, w)

	// Initial event.
	wc.AssertOneChange()

	// Change the state addresses and check that we get a notification
	expectServerAddrs[0][0].Value = "0.1.99.99"

	err = s.state.SetAPIHostPorts(expectServerAddrs)
	c.Assert(err, gc.IsNil)

	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}
