// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package testing

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/watcher"
)

type APIAddresserSuite struct {
	state     *state.State
	facade APIAddresserFacade
}

type APIAddresserFacade interface {
	APIAddresses() ([]string, error)
	CACert() ([]byte, error)
	APIHostPorts() ([][]instance.HostPort, error)
	WatchAPIHostPorts() (watcher.NotifyWatcher, error)
}

func NewAPIAddresserSuite(st *state.State, facade APIAddresserFacade) *APIAddresserSuite {
	return &APIAddresserSuite{
		state:     st,
		facade: facade,
	}
}

func (s *APIAddresserSuite) SetUpSuite(c *gc.C) {
}

func (s *APIAddresserSuite) TearDownSuite(c *gc.C) {
}

func (s *APIAddresserSuite) SetUpTest(c *gc.C) {
}

func (s *APIAddresserSuite) TearDownTest(c *gc.C) {
}

func (s *APIAddresserSuite) TestAPIAddresses(c *gc.C) {
	apiAddresses, err := s.state.APIAddressesFromMachines()
	c.Assert(err, gc.IsNil)

	addresses, err := s.facade.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addresses, gc.DeepEquals, apiAddresses)
}

func (s *APIAddresserSuite) TestAPIHostPorts(c *gc.C) {
	expectServerAddrs := [][]instance.HostPort{{{
		Address: instance.NewAddress("0.1.2.24"),
		Port:    999,
	}, {
		Address: instance.NewAddress("example.com"),
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

func (s *APIAddresserSuite) TestCACert(c *gc.C) {
	caCert, err := s.facade.CACert()
	c.Assert(err, gc.IsNil)
	c.Assert(caCert, gc.DeepEquals, s.state.CACert())
}
