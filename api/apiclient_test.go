// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"net"
	"sync/atomic"

	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/parallel"
	"golang.org/x/net/websocket"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/version"
)

type apiclientSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&apiclientSuite{})

func (s *apiclientSuite) TestOpenFailsIfUsernameAndUseMacaroon(c *gc.C) {
	info := s.APIInfo(c)
	info.Tag = names.NewUserTag("foobar")
	info.UseMacaroons = true
	_, err := api.Open(info, api.DialOpts{})
	c.Assert(err, gc.ErrorMatches, "open should specifiy UseMacaroons or a username & password. Not both")
}

func (s *apiclientSuite) TestConnectWebsocketToEnv(c *gc.C) {
	info := s.APIInfo(c)
	conn, _, err := api.ConnectWebsocket(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()
	assertConnAddrForEnv(c, conn, info.Addrs[0], s.State.EnvironUUID(), "/api")
}

func (s *apiclientSuite) TestConnectWebsocketToRoot(c *gc.C) {
	info := s.APIInfo(c)
	info.EnvironTag = names.NewEnvironTag("")
	conn, _, err := api.ConnectWebsocket(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()
	assertConnAddrForRoot(c, conn, info.Addrs[0])
}

func (s *apiclientSuite) TestConnectWebsocketMultiple(c *gc.C) {
	// Create a socket that proxies to the API server.
	info := s.APIInfo(c)
	serverAddr := info.Addrs[0]
	proxy := testing.NewTCPProxy(c, serverAddr)
	defer proxy.Close()

	// Check that we can use the proxy to connect.
	info.Addrs = []string{proxy.Addr()}
	conn, _, err := api.ConnectWebsocket(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	conn.Close()
	assertConnAddrForEnv(c, conn, proxy.Addr(), s.State.EnvironUUID(), "/api")

	// Now break Addrs[0], and ensure that Addrs[1]
	// is successfully connected to.
	proxy.Close()
	info.Addrs = []string{proxy.Addr(), serverAddr}
	conn, _, err = api.ConnectWebsocket(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	conn.Close()
	assertConnAddrForEnv(c, conn, serverAddr, s.State.EnvironUUID(), "/api")
}

func (s *apiclientSuite) TestConnectWebsocketMultipleError(c *gc.C) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, jc.ErrorIsNil)
	defer listener.Close()
	// count holds the number of times we've accepted a connection.
	var count int32
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			atomic.AddInt32(&count, 1)
			client.Close()
		}
	}()
	info := s.APIInfo(c)
	addr := listener.Addr().String()
	info.Addrs = []string{addr, addr, addr}
	_, _, err = api.ConnectWebsocket(info, api.DialOpts{})
	c.Assert(err, gc.ErrorMatches, `unable to connect to API: websocket.Dial wss://.*/environment/[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}/api: .*`)
	c.Assert(count, gc.Equals, int32(3))
}

func (s *apiclientSuite) TestOpen(c *gc.C) {
	info := s.APIInfo(c)
	st, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	c.Assert(st.Addr(), gc.Equals, info.Addrs[0])
	envTag, err := st.EnvironTag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envTag, gc.Equals, s.State.EnvironTag())

	remoteVersion, versionSet := st.ServerVersion()
	c.Assert(versionSet, jc.IsTrue)
	c.Assert(remoteVersion, gc.Equals, version.Current)
}

func (s *apiclientSuite) TestOpenHonorsEnvironTag(c *gc.C) {
	info := s.APIInfo(c)

	// TODO(jam): 2014-06-05 http://pad.lv/1326802
	// we want to test this eventually, but for now s.APIInfo uses
	// conn.StateInfo() which doesn't know about EnvironTag.
	// c.Check(info.EnvironTag, gc.Equals, env.Tag())
	// c.Assert(info.EnvironTag, gc.Not(gc.Equals), "")

	// We start by ensuring we have an invalid tag, and Open should fail.
	info.EnvironTag = names.NewEnvironTag("bad-tag")
	_, err := api.Open(info, api.DialOpts{})
	c.Check(err, gc.ErrorMatches, `unknown environment: "bad-tag"`)
	c.Check(params.ErrCode(err), gc.Equals, params.CodeNotFound)

	// Now set it to the right tag, and we should succeed.
	info.EnvironTag = s.State.EnvironTag()
	st, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	st.Close()

	// Backwards compatibility, we should succeed if we do not set an
	// environ tag
	info.EnvironTag = names.NewEnvironTag("")
	st, err = api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	st.Close()
}

func (s *apiclientSuite) TestServerRoot(c *gc.C) {
	url := api.ServerRoot(s.APIState.Client())
	c.Assert(url, gc.Matches, "https://localhost:[0-9]+")
}

func (s *apiclientSuite) TestDialWebsocketStopped(c *gc.C) {
	stopped := make(chan struct{})
	f := api.NewWebsocketDialer(nil, api.DialOpts{})
	close(stopped)
	result, err := f(stopped)
	c.Assert(err, gc.Equals, parallel.ErrStopped)
	c.Assert(result, gc.IsNil)
}

func assertConnAddrForEnv(c *gc.C, conn *websocket.Conn, addr, envUUID, tail string) {
	c.Assert(conn.RemoteAddr(), gc.Matches, "^wss://"+addr+"/environment/"+envUUID+tail+"$")
}

func assertConnAddrForRoot(c *gc.C, conn *websocket.Conn, addr string) {
	c.Assert(conn.RemoteAddr(), gc.Matches, "^wss://"+addr+"/$")
}
