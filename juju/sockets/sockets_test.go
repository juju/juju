// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sockets_test

import (
	"crypto/tls"
	"crypto/x509"
	"net/rpc"
	"path/filepath"
	"testing"

	"github.com/juju/tc"

	jujutesting "github.com/juju/juju/internal/testing"
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
		Address: "127.0.0.1:32134",
		Network: "tcp",
	}
	s.testConn(c, socketDesc, socketDesc)
}

func (s *SocketSuite) TestAbstractDomain(c *tc.C) {
	socketDesc := sockets.Socket{
		Address: "@hello-juju",
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
	serverSocketDesc := sockets.Socket{
		Address: "127.0.0.1:32135",
		Network: "tcp",
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{*jujutesting.ServerTLSCert},
		},
	}
	clientSocketDesc := sockets.Socket{
		Address: "127.0.0.1:32135",
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
