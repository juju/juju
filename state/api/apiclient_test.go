// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/juju/names"
	"github.com/juju/utils/parallel"
	gc "launchpad.net/gocheck"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/api/params"
	coretesting "github.com/juju/juju/testing"
)

type apiclientSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&apiclientSuite{})

type websocketSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&websocketSuite{})

func (s *apiclientSuite) TestOpenPrefersLocalhostIfPresent(c *gc.C) {
	// Create a socket that proxies to the API server though our localhost address.
	info := s.APIInfo(c)
	serverAddr := info.Addrs[0]
	server, err := net.Dial("tcp", serverAddr)
	c.Assert(err, gc.IsNil)
	defer server.Close()
	listener, err := net.Listen("tcp", "localhost:0")
	c.Assert(err, gc.IsNil)
	defer listener.Close()
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			go io.Copy(client, server)
			go io.Copy(server, client)
		}
	}()

	// Check that we are using our working address to connect
	listenerAddress := listener.Addr().String()
	// listenAddress contains the actual IP address, but APIHostPorts
	// is going to report localhost, so just find the port
	_, port, err := net.SplitHostPort(listenerAddress)
	c.Check(err, gc.IsNil)
	portNum, err := strconv.Atoi(port)
	c.Check(err, gc.IsNil)
	expectedHostPort := fmt.Sprintf("localhost:%d", portNum)
	info.Addrs = []string{"fakeAddress:1", "fakeAddress:1", expectedHostPort}
	st, err := api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	defer st.Close()
	c.Assert(st.Addr(), gc.Equals, expectedHostPort)
}

func (s *apiclientSuite) TestOpenMultiple(c *gc.C) {
	// Create a socket that proxies to the API server.
	info := s.APIInfo(c)
	serverAddr := info.Addrs[0]
	server, err := net.Dial("tcp", serverAddr)
	c.Assert(err, gc.IsNil)
	defer server.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, gc.IsNil)
	defer listener.Close()
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			go io.Copy(client, server)
			go io.Copy(server, client)
		}
	}()

	// Check that we can use the proxy to connect.
	proxyAddr := listener.Addr().String()
	info.Addrs = []string{proxyAddr}
	st, err := api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	defer st.Close()
	c.Assert(st.Addr(), gc.Equals, proxyAddr)

	// Now break Addrs[0], and ensure that Addrs[1]
	// is successfully connected to.
	info.Addrs = []string{proxyAddr, serverAddr}
	listener.Close()
	st, err = api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	defer st.Close()
	c.Assert(st.Addr(), gc.Equals, serverAddr)
}

func (s *apiclientSuite) TestOpenMultipleError(c *gc.C) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, gc.IsNil)
	defer listener.Close()
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			client.Close()
		}
	}()
	info := s.APIInfo(c)
	addr := listener.Addr().String()
	info.Addrs = []string{addr, addr, addr}
	_, err = api.Open(info, api.DialOpts{})
	c.Assert(err, gc.ErrorMatches, `unable to connect to "wss://.*/"`)
}

func (s *apiclientSuite) TestOpenPassesEnvironTag(c *gc.C) {
	info := s.APIInfo(c)
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	// TODO(jam): 2014-06-05 http://pad.lv/1326802
	// we want to test this eventually, but for now s.APIInfo uses
	// conn.StateInfo() which doesn't know about EnvironTag.
	// c.Check(info.EnvironTag, gc.Equals, env.Tag())
	// c.Assert(info.EnvironTag, gc.Not(gc.Equals), "")
	// We start by ensuring we have an invalid tag, and Open should fail.
	info.EnvironTag = names.NewEnvironTag("bad-tag")
	_, err = api.Open(info, api.DialOpts{})
	c.Check(err, gc.ErrorMatches, `unknown environment: "bad-tag"`)
	c.Check(params.ErrCode(err), gc.Equals, params.CodeNotFound)
	// Now set it to the right tag, and we should succeed.
	info.EnvironTag = env.Tag()
	st, err := api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	st.Close()
	// Backwards compatibility, we should succeed if we do not set an
	// environ tag
	info.EnvironTag = nil
	st, err = api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	st.Close()
}

func (s *apiclientSuite) TestDialWebsocketStopped(c *gc.C) {
	stopped := make(chan struct{})
	f := api.NewWebsocketDialer(nil, api.DialOpts{})
	close(stopped)
	result, err := f(stopped)
	c.Assert(err, gc.Equals, parallel.ErrStopped)
	c.Assert(result, gc.IsNil)
}

func (*websocketSuite) TestSetUpWebsocketConfig(c *gc.C) {
	conf, err := api.SetUpWebsocket("0.1.2.3:1234", "", nil)
	c.Assert(err, gc.IsNil)
	c.Check(conf.Location.String(), gc.Equals, "wss://0.1.2.3:1234/")
	c.Check(conf.Origin.String(), gc.Equals, "http://localhost/")
}

func (*websocketSuite) TestSetUpWebsocketConfigHandlesEnvironUUID(c *gc.C) {
	conf, err := api.SetUpWebsocket("0.1.2.3:1234", "dead-beef-1234", nil)
	c.Assert(err, gc.IsNil)
	c.Check(conf.Location.String(), gc.Equals, "wss://0.1.2.3:1234/environment/dead-beef-1234/api")
	c.Check(conf.Origin.String(), gc.Equals, "http://localhost/")
}
