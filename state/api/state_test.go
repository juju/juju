// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api"
	coretesting "launchpad.net/juju-core/testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type stateSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&stateSuite{})

type slideSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&slideSuite{})

func (s *stateSuite) TestCloseMultipleOk(c *gc.C) {
	c.Assert(s.APIState.Close(), gc.IsNil)
	c.Assert(s.APIState.Close(), gc.IsNil)
	c.Assert(s.APIState.Close(), gc.IsNil)
}

func (s *stateSuite) TestAPIHostPortsAlwaysIncludesTheConnection(c *gc.C) {
	hostportslist := s.APIState.APIHostPorts()
	c.Check(hostportslist, gc.HasLen, 1)
	serverhostports := hostportslist[0]
	c.Check(serverhostports, gc.HasLen, 1)
	// the other addresses, but always see this one as well.
	info := s.APIInfo(c)
	// We intentionally set this to invalid values
	badValue := instance.HostPort{instance.Address{
		Value:        "0.1.2.3",
		Type:         instance.Ipv4Address,
		NetworkName:  "",
		NetworkScope: instance.NetworkMachineLocal,
	}, 1234}
	badServer := []instance.HostPort{badValue}
	s.State.SetAPIHostPorts([][]instance.HostPort{badServer})
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	hostports := apistate.APIHostPorts()
	c.Check(hostports, gc.DeepEquals, [][]instance.HostPort{serverhostports, badServer})
}

func (s *stateSuite) TestAPIHostPortsMovesConnectedValueFirst(c *gc.C) {
	hostportslist := s.APIState.APIHostPorts()
	c.Check(hostportslist, gc.HasLen, 1)
	serverhostports := hostportslist[0]
	c.Check(serverhostports, gc.HasLen, 1)
	goodAddress := serverhostports[0]
	// the other addresses, but always see this one as well.
	info := s.APIInfo(c)
	// We intentionally set this to invalid values
	badValue := instance.HostPort{instance.Address{
		Value:        "0.1.2.3",
		Type:         instance.Ipv4Address,
		NetworkName:  "",
		NetworkScope: instance.NetworkMachineLocal,
	}, 1234}
	badServer := []instance.HostPort{badValue}
	extraAddress := instance.HostPort{instance.Address{
		Value:        "0.1.2.4",
		Type:         instance.Ipv4Address,
		NetworkName:  "",
		NetworkScope: instance.NetworkMachineLocal,
	}, 5678}
	extraAddress2 := instance.HostPort{instance.Address{
		Value:        "0.1.2.1",
		Type:         instance.Ipv4Address,
		NetworkName:  "",
		NetworkScope: instance.NetworkMachineLocal,
	}, 9012}
	serverExtra := []instance.HostPort{extraAddress, goodAddress, extraAddress2}
	current := [][]instance.HostPort{badServer, serverExtra}
	s.State.SetAPIHostPorts(current)
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	hostports := apistate.APIHostPorts()
	// We should have rotate the server we connected to as the first item,
	// and the address of that server as the first address
	sortedServer := []instance.HostPort{goodAddress, extraAddress, extraAddress2}
	expected := [][]instance.HostPort{sortedServer, badServer}
	c.Check(hostports, gc.DeepEquals, expected)
}

var exampleHostPorts = []instance.HostPort{
	{
		Address: instance.Address{
			Value:        "0.1.2.3",
			Type:         instance.Ipv4Address,
			NetworkName:  "",
			NetworkScope: instance.NetworkUnknown,
		}, Port: 1234,
	}, {
		Address: instance.Address{
			Value:        "0.1.2.4",
			Type:         instance.Ipv4Address,
			NetworkName:  "",
			NetworkScope: instance.NetworkUnknown,
		}, Port: 5678,
	}, {
		Address: instance.Address{
			Value:        "0.1.2.1",
			Type:         instance.Ipv4Address,
			NetworkName:  "",
			NetworkScope: instance.NetworkUnknown,
		}, Port: 9012,
	}, {
		Address: instance.Address{
			Value:        "0.1.9.1",
			Type:         instance.Ipv4Address,
			NetworkName:  "",
			NetworkScope: instance.NetworkUnknown,
		}, Port: 8888,
	},
}

func (s *slideSuite) TestSlideToFrontNoOp(c *gc.C) {
	servers := [][]instance.HostPort{
		{exampleHostPorts[0]},
		{exampleHostPorts[1]},
	}
	// order should not have changed
	expected := [][]instance.HostPort{
		{exampleHostPorts[0]},
		{exampleHostPorts[1]},
	}
	api.SlideAddressToFront(servers, 0, 0)
	c.Check(servers, gc.DeepEquals, expected)
}

func (s *slideSuite) TestSlideToFrontAddress(c *gc.C) {
	servers := [][]instance.HostPort{
		{exampleHostPorts[0], exampleHostPorts[1], exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	// server order should not change, but ports should be switched
	expected := [][]instance.HostPort{
		{exampleHostPorts[1], exampleHostPorts[0], exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	api.SlideAddressToFront(servers, 0, 1)
	c.Check(servers, gc.DeepEquals, expected)
}

func (s *slideSuite) TestSlideToFrontServer(c *gc.C) {
	servers := [][]instance.HostPort{
		{exampleHostPorts[0], exampleHostPorts[1]},
		{exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	// server 1 should be slid to the front
	expected := [][]instance.HostPort{
		{exampleHostPorts[2]},
		{exampleHostPorts[0], exampleHostPorts[1]},
		{exampleHostPorts[3]},
	}
	api.SlideAddressToFront(servers, 1, 0)
	c.Check(servers, gc.DeepEquals, expected)
}

func (s *slideSuite) TestSlideToFrontBoth(c *gc.C) {
	servers := [][]instance.HostPort{
		{exampleHostPorts[0]},
		{exampleHostPorts[1], exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	// server 1 should be slid to the front
	expected := [][]instance.HostPort{
		{exampleHostPorts[2], exampleHostPorts[1]},
		{exampleHostPorts[0]},
		{exampleHostPorts[3]},
	}
	api.SlideAddressToFront(servers, 1, 1)
	c.Check(servers, gc.DeepEquals, expected)
}
