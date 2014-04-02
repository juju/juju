// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"io"
	"net"

	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils/parallel"
)

type apiclientSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&apiclientSuite{})

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
	c.Assert(err, gc.ErrorMatches, `timed out connecting to "wss://.*/"`)
}

func (s *apiclientSuite) TestDialWebsocketStopped(c *gc.C) {
	stopped := make(chan struct{})
	f := api.NewWebsocketDialer(nil, api.DialOpts{})
	close(stopped)
	result, err := f(stopped)
	c.Assert(err, gc.Equals, parallel.ErrStopped)
	c.Assert(result, gc.IsNil)
}
