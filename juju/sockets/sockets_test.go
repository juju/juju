// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sockets_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/rpc"
	"path/filepath"
	"testing"

	"github.com/juju/tc"

	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/juju/sockets"
)

type RpcCaller func(string, *string) error

func (f RpcCaller) TestCall(arg string, reply *string) error {
	return f(arg, reply)
}

type SocketSuite struct {
}

func TestSocketSuite(t *testing.T) {
	tc.Run(t, &SocketSuite{})
}

func (s *SocketSuite) TestTCP(c *tc.C) {
	socketDesc := sockets.Socket{
		Address: fmt.Sprintf("127.0.0.1:%d", randomTCPPort(c)),
		Network: "tcp",
	}
	s.testConn(c, socketDesc, socketDesc)
}

func (s *SocketSuite) TestAbstractDomain(c *tc.C) {
	id := uuid.MustNewUUID()
	socketDesc := sockets.Socket{
		Address: "@" + id.String(),
		Network: "unix",
	}
	s.testConn(c, socketDesc, socketDesc)
}

func (s *SocketSuite) TestUNIXSocket(c *tc.C) {
	socketDir := c.MkDir()
	socketPath := filepath.Join(socketDir, "a.socket")
	socketDesc := sockets.Socket{
		Address: socketPath,
		Network: "unix",
	}
	s.testConn(c, socketDesc, socketDesc)
}

func (s *SocketSuite) TestTLSOverTCP(c *tc.C) {
	roots := x509.NewCertPool()
	roots.AddCert(jujutesting.CACertX509)
	addr := fmt.Sprintf("127.0.0.1:%d", randomTCPPort(c))
	serverSocketDesc := sockets.Socket{
		Address: addr,
		Network: "tcp",
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{*jujutesting.ServerTLSCert},
		},
	}
	clientSocketDesc := sockets.Socket{
		Address: addr,
		Network: "tcp",
		TLSConfig: &tls.Config{
			RootCAs:            roots,
			InsecureSkipVerify: true,
		},
	}
	s.testConn(c, serverSocketDesc, clientSocketDesc)
}

func (s *SocketSuite) testConn(c *tc.C, serverSocketDesc, clientSocketDesc sockets.Socket) {
	l, err := sockets.Listen(serverSocketDesc)
	c.Assert(err, tc.ErrorIsNil)

	srv := rpc.Server{}
	called := false
	err = srv.Register(RpcCaller(func(arg string, reply *string) error {
		called = true
		*reply = arg
		return nil
	}))
	c.Assert(err, tc.ErrorIsNil)

	go func() {
		cconn, err := sockets.Dial(clientSocketDesc)
		c.Assert(err, tc.ErrorIsNil)
		rep := ""
		err = cconn.Call("RpcCaller.TestCall", "hello", &rep)
		c.Check(err, tc.ErrorIsNil)
		c.Check(rep, tc.Equals, "hello")
		err = cconn.Close()
		c.Assert(err, tc.ErrorIsNil)
	}()

	sconn, err := l.Accept()
	c.Assert(err, tc.ErrorIsNil)
	srv.ServeConn(sconn)
	err = l.Close()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}

func randomTCPPort(c *tc.C) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, tc.ErrorIsNil)
	addr := l.Addr()
	err = l.Close()
	c.Assert(err, tc.ErrorIsNil)
	return addr.(*net.TCPAddr).Port
}
