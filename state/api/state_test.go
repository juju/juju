// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/api"
	coretesting "github.com/juju/juju/testing"
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
	badValue := network.HostPort{network.Address{
		Value:       "0.1.2.3",
		Type:        network.IPv4Address,
		NetworkName: "",
		Scope:       network.ScopeMachineLocal,
	}, 1234}
	badServer := []network.HostPort{badValue}
	s.State.SetAPIHostPorts([][]network.HostPort{badServer})
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	defer apistate.Close()
	hostports := apistate.APIHostPorts()
	c.Check(hostports, gc.DeepEquals, [][]network.HostPort{serverhostports, badServer})
}

func (s *stateSuite) TestLoginSetsEnvironTag(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	info := s.APIInfo(c)
	tag := info.Tag
	password := info.Password
	info.Tag = ""
	info.Password = ""
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	defer apistate.Close()
	// We haven't called Login yet, so the EnvironTag shouldn't be set.
	c.Check(apistate.EnvironTag(), gc.Equals, "")
	err = apistate.Login(tag, password, "")
	c.Assert(err, gc.IsNil)
	// Now that we've logged in, EnvironTag should be updated correctly.
	c.Check(apistate.EnvironTag(), gc.Equals, env.Tag())
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
	badValue := network.HostPort{network.Address{
		Value:       "0.1.2.3",
		Type:        network.IPv4Address,
		NetworkName: "",
		Scope:       network.ScopeMachineLocal,
	}, 1234}
	badServer := []network.HostPort{badValue}
	extraAddress := network.HostPort{network.Address{
		Value:       "0.1.2.4",
		Type:        network.IPv4Address,
		NetworkName: "",
		Scope:       network.ScopeMachineLocal,
	}, 5678}
	extraAddress2 := network.HostPort{network.Address{
		Value:       "0.1.2.1",
		Type:        network.IPv4Address,
		NetworkName: "",
		Scope:       network.ScopeMachineLocal,
	}, 9012}
	serverExtra := []network.HostPort{extraAddress, goodAddress, extraAddress2}
	current := [][]network.HostPort{badServer, serverExtra}
	s.State.SetAPIHostPorts(current)
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	defer apistate.Close()
	hostports := apistate.APIHostPorts()
	// We should have rotate the server we connected to as the first item,
	// and the address of that server as the first address
	sortedServer := []network.HostPort{goodAddress, extraAddress, extraAddress2}
	expected := [][]network.HostPort{sortedServer, badServer}
	c.Check(hostports, gc.DeepEquals, expected)
}

var exampleHostPorts = []network.HostPort{
	{
		Address: network.Address{
			Value:       "0.1.2.3",
			Type:        network.IPv4Address,
			NetworkName: "",
			Scope:       network.ScopeUnknown,
		}, Port: 1234,
	}, {
		Address: network.Address{
			Value:       "0.1.2.4",
			Type:        network.IPv4Address,
			NetworkName: "",
			Scope:       network.ScopeUnknown,
		}, Port: 5678,
	}, {
		Address: network.Address{
			Value:       "0.1.2.1",
			Type:        network.IPv4Address,
			NetworkName: "",
			Scope:       network.ScopeUnknown,
		}, Port: 9012,
	}, {
		Address: network.Address{
			Value:       "0.1.9.1",
			Type:        network.IPv4Address,
			NetworkName: "",
			Scope:       network.ScopeUnknown,
		}, Port: 8888,
	},
}

func (s *slideSuite) TestSlideToFrontNoOp(c *gc.C) {
	servers := [][]network.HostPort{
		{exampleHostPorts[0]},
		{exampleHostPorts[1]},
	}
	// order should not have changed
	expected := [][]network.HostPort{
		{exampleHostPorts[0]},
		{exampleHostPorts[1]},
	}
	api.SlideAddressToFront(servers, 0, 0)
	c.Check(servers, gc.DeepEquals, expected)
}

func (s *slideSuite) TestSlideToFrontAddress(c *gc.C) {
	servers := [][]network.HostPort{
		{exampleHostPorts[0], exampleHostPorts[1], exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	// server order should not change, but ports should be switched
	expected := [][]network.HostPort{
		{exampleHostPorts[1], exampleHostPorts[0], exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	api.SlideAddressToFront(servers, 0, 1)
	c.Check(servers, gc.DeepEquals, expected)
}

func (s *slideSuite) TestSlideToFrontServer(c *gc.C) {
	servers := [][]network.HostPort{
		{exampleHostPorts[0], exampleHostPorts[1]},
		{exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	// server 1 should be slid to the front
	expected := [][]network.HostPort{
		{exampleHostPorts[2]},
		{exampleHostPorts[0], exampleHostPorts[1]},
		{exampleHostPorts[3]},
	}
	api.SlideAddressToFront(servers, 1, 0)
	c.Check(servers, gc.DeepEquals, expected)
}

func (s *slideSuite) TestSlideToFrontBoth(c *gc.C) {
	servers := [][]network.HostPort{
		{exampleHostPorts[0]},
		{exampleHostPorts[1], exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	// server 1 should be slid to the front
	expected := [][]network.HostPort{
		{exampleHostPorts[2], exampleHostPorts[1]},
		{exampleHostPorts[0]},
		{exampleHostPorts[3]},
	}
	api.SlideAddressToFront(servers, 1, 1)
	c.Check(servers, gc.DeepEquals, expected)
}
