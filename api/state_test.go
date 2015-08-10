// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	stdtesting "testing"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
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

// OpenAPIWithoutLogin connects to the API and returns an api.State without
// actually calling st.Login already. The returned strings are the "tag" and
// "password" that we would have used to login.
func (s *stateSuite) OpenAPIWithoutLogin(c *gc.C) (api.Connection, string, string) {
	info := s.APIInfo(c)
	tag := info.Tag
	password := info.Password
	info.Tag = nil
	info.Password = ""
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	return apistate, tag.String(), password
}

func (s *stateSuite) TestAPIHostPortsAlwaysIncludesTheConnection(c *gc.C) {
	hostportslist := s.APIState.APIHostPorts()
	c.Check(hostportslist, gc.HasLen, 1)
	serverhostports := hostportslist[0]
	c.Check(serverhostports, gc.HasLen, 1)
	// the other addresses, but always see this one as well.
	info := s.APIInfo(c)
	// We intentionally set this to invalid values
	badServer := network.NewHostPorts(1234, "0.1.2.3")
	badServer[0].Scope = network.ScopeMachineLocal
	s.State.SetAPIHostPorts([][]network.HostPort{badServer})
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apistate.Close()
	hostports := apistate.APIHostPorts()
	c.Check(hostports, gc.DeepEquals, [][]network.HostPort{
		serverhostports,
		badServer,
	})
}

func (s *stateSuite) TestLoginSetsEnvironTag(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	apistate, tag, password := s.OpenAPIWithoutLogin(c)
	defer apistate.Close()
	// We haven't called Login yet, so the EnvironTag shouldn't be set.
	envTag, err := apistate.EnvironTag()
	c.Check(err, gc.ErrorMatches, `"" is not a valid tag`)
	c.Check(envTag, gc.Equals, names.EnvironTag{})
	err = apistate.Login(tag, password, "")
	c.Assert(err, jc.ErrorIsNil)
	// Now that we've logged in, EnvironTag should be updated correctly.
	envTag, err = apistate.EnvironTag()
	c.Check(err, jc.ErrorIsNil)
	c.Check(envTag, gc.Equals, env.EnvironTag())
	// The server tag is also set, and since the environment is the
	// state server environment, the uuid is the same.
	srvTag, err := apistate.ServerTag()
	c.Check(err, jc.ErrorIsNil)
	c.Check(srvTag, gc.Equals, env.EnvironTag())
}

func (s *stateSuite) TestLoginTracksFacadeVersions(c *gc.C) {
	apistate, tag, password := s.OpenAPIWithoutLogin(c)
	defer apistate.Close()
	// We haven't called Login yet, so the Facade Versions should be empty
	c.Check(apistate.AllFacadeVersions(), gc.HasLen, 0)
	err := apistate.Login(tag, password, "")
	c.Assert(err, jc.ErrorIsNil)
	// Now that we've logged in, AllFacadeVersions should be updated.
	allVersions := apistate.AllFacadeVersions()
	c.Check(allVersions, gc.Not(gc.HasLen), 0)
	// For sanity checking, ensure that we have a v2 of the Client facade
	c.Assert(allVersions["Client"], gc.Not(gc.HasLen), 0)
	c.Check(allVersions["Client"][0], gc.Equals, 0)
}

func (s *stateSuite) TestAllFacadeVersionsSafeFromMutation(c *gc.C) {
	allVersions := s.APIState.AllFacadeVersions()
	clients := allVersions["Client"]
	origClients := make([]int, len(clients))
	copy(origClients, clients)
	// Mutating the dict should not affect the cached versions
	allVersions["Client"] = append(allVersions["Client"], 2597)
	newVersions := s.APIState.AllFacadeVersions()
	newClientVers := newVersions["Client"]
	c.Check(newClientVers, gc.DeepEquals, origClients)
	c.Check(newClientVers[len(newClientVers)-1], gc.Not(gc.Equals), 2597)
}

func (s *stateSuite) TestBestFacadeVersion(c *gc.C) {
	c.Check(s.APIState.BestFacadeVersion("Client"), gc.Equals, 0)
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
	serverExtra := []network.HostPort{
		extraAddress, goodAddress, extraAddress2,
	}
	current := [][]network.HostPort{badServer, serverExtra}
	s.State.SetAPIHostPorts(current)
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apistate.Close()
	hostports := apistate.APIHostPorts()
	// We should have rotate the server we connected to as the first item,
	// and the address of that server as the first address
	sortedServer := []network.HostPort{
		goodAddress, extraAddress, extraAddress2,
	}
	expected := [][]network.HostPort{sortedServer, badServer}
	c.Check(hostports, gc.DeepEquals, expected)
}

var exampleHostPorts = []network.HostPort{{
	Address: network.NewAddress("0.1.2.3"),
	Port:    1234,
}, {
	Address: network.NewAddress("0.1.2.4"),
	Port:    5678,
}, {
	Address: network.NewAddress("0.1.2.1"),
	Port:    9012,
}, {
	Address: network.NewAddress("0.1.9.1"),
	Port:    8888,
}}

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
