// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttransport_test

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/api"
	coretesting "github.com/juju/juju/v3/testing"
	"github.com/juju/juju/v3/worker/raft/rafttransport"
)

type HandlerSuite struct {
	testing.IsolationSuite
	connections chan net.Conn
	handler     *rafttransport.Handler
	server      *httptest.Server
}

var _ = gc.Suite(&HandlerSuite{})

func (s *HandlerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.connections = make(chan net.Conn)
	s.handler = rafttransport.NewHandler(s.connections, nil)
	s.server = httptest.NewTLSServer(s.handler)
	s.AddCleanup(func(c *gc.C) {
		s.server.Close()
	})
}

func (s *HandlerSuite) TestHandler(c *gc.C) {
	u, err := url.Parse(s.server.URL)
	c.Assert(err, jc.ErrorIsNil)
	dialRaw := func(addr raft.ServerAddress, timeout time.Duration) (net.Conn, error) {
		tlsConfig := s.server.Client().Transport.(*http.Transport).TLSClientConfig
		return tls.Dial("tcp", u.Host, tlsConfig)
	}
	dialer := rafttransport.Dialer{
		APIInfo: &api.Info{},
		Path:    "/raft",
		DialRaw: dialRaw,
	}
	clientConn, err := dialer.Dial("", 0)
	c.Assert(err, jc.ErrorIsNil)
	defer clientConn.Close()

	var serverConn net.Conn
	select {
	case conn := <-s.connections:
		serverConn = conn
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for server connection")
	}
	defer serverConn.Close()

	payload := "hello, server!"
	n, err := clientConn.Write([]byte(payload))
	c.Assert(err, jc.ErrorIsNil)

	read := make([]byte, n)
	n, err = serverConn.Read(read)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, len(payload))
	c.Assert(string(read), gc.Equals, payload)
}
