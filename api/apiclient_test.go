// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"

	"golang.org/x/net/websocket"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/parallel"
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

func (s *apiclientSuite) TestConnectToEnv(c *gc.C) {
	info := s.APIInfo(c)
	conn, err := api.Connect(info, "", nil, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()
	assertConnAddrForEnv(c, conn, info.Addrs[0], s.State.EnvironUUID(), "/api")
}

func (s *apiclientSuite) TestConnectToEnvWithPathTail(c *gc.C) {
	info := s.APIInfo(c)
	conn, err := api.Connect(info, "/log", nil, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()
	assertConnAddrForEnv(c, conn, info.Addrs[0], s.State.EnvironUUID(), "/log")
}

func (s *apiclientSuite) TestConnectToRoot(c *gc.C) {
	info := s.APIInfo(c)
	info.EnvironTag = names.NewEnvironTag("")
	conn, err := api.Connect(info, "", nil, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()
	assertConnAddrForRoot(c, conn, info.Addrs[0])
}

func (s *apiclientSuite) TestConnectWithHeader(c *gc.C) {
	var seenCfg *websocket.Config
	fakeNewDialer := func(cfg *websocket.Config, _ api.DialOpts) func(<-chan struct{}) (io.Closer, error) {
		seenCfg = cfg
		return func(<-chan struct{}) (io.Closer, error) {
			return nil, errors.New("fake")
		}
	}
	s.PatchValue(api.NewWebsocketDialerPtr, fakeNewDialer)

	header := utils.BasicAuthHeader("foo", "bar")
	api.Connect(s.APIInfo(c), "", header, api.DialOpts{}) // Return values not important here
	c.Assert(seenCfg, gc.NotNil)
	c.Assert(seenCfg.Header, gc.DeepEquals, header)
}

func (s *apiclientSuite) TestConnectRequiresTailStartsWithSlash(c *gc.C) {
	_, err := api.Connect(s.APIInfo(c), "foo", nil, api.DialOpts{})
	c.Assert(err, gc.ErrorMatches, `path tail must start with "/"`)
}

func (s *apiclientSuite) TestConnectPrefersLocalhostIfPresent(c *gc.C) {
	// Create a socket that proxies to the API server though our localhost address.
	info := s.APIInfo(c)
	serverAddr := info.Addrs[0]
	server, err := net.Dial("tcp", serverAddr)
	c.Assert(err, jc.ErrorIsNil)
	defer server.Close()
	listener, err := net.Listen("tcp", "localhost:0")
	c.Assert(err, jc.ErrorIsNil)
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
	c.Check(err, jc.ErrorIsNil)
	portNum, err := strconv.Atoi(port)
	c.Check(err, jc.ErrorIsNil)
	expectedHostPort := fmt.Sprintf("localhost:%d", portNum)
	info.Addrs = []string{"fakeAddress:1", "fakeAddress:1", expectedHostPort}
	conn, err := api.Connect(info, "/api", nil, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()
	assertConnAddrForEnv(c, conn, expectedHostPort, s.State.EnvironUUID(), "/api")
}

func (s *apiclientSuite) TestConnectMultiple(c *gc.C) {
	// Create a socket that proxies to the API server.
	info := s.APIInfo(c)
	serverAddr := info.Addrs[0]
	server, err := net.Dial("tcp", serverAddr)
	c.Assert(err, jc.ErrorIsNil)
	defer server.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, jc.ErrorIsNil)
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
	conn, err := api.Connect(info, "/api", nil, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	conn.Close()
	assertConnAddrForEnv(c, conn, proxyAddr, s.State.EnvironUUID(), "/api")

	// Now break Addrs[0], and ensure that Addrs[1]
	// is successfully connected to.
	info.Addrs = []string{proxyAddr, serverAddr}
	listener.Close()
	conn, err = api.Connect(info, "/api", nil, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	conn.Close()
	assertConnAddrForEnv(c, conn, serverAddr, s.State.EnvironUUID(), "/api")
}

func (s *apiclientSuite) TestConnectMultipleError(c *gc.C) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, jc.ErrorIsNil)
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
	_, err = api.Connect(info, "/api", nil, api.DialOpts{})
	c.Assert(err, gc.ErrorMatches, `unable to connect to "wss://.*/environment/[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}/api"`)
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
	c.Assert(remoteVersion, gc.Equals, version.Current.Number)
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
