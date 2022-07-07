// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sockets_test

import (
	"crypto/tls"
	"crypto/x509"
	"net/rpc"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/sockets"
	jujutesting "github.com/juju/juju/testing"
)

type RpcCaller func(string, *string) error

func (f RpcCaller) TestCall(arg string, reply *string) error {
	return f(arg, reply)
}

type SocketSuite struct {
}

var _ = gc.Suite(&SocketSuite{})

func (s *SocketSuite) TestTCP(c *gc.C) {
	socketDesc := sockets.Socket{
		Address: "127.0.0.1:32134",
		Network: "tcp",
	}
	s.testConn(c, socketDesc, socketDesc)
}

func (s *SocketSuite) TestAbstractDomain(c *gc.C) {
	socketDesc := sockets.Socket{
		Address: "@hello-juju",
		Network: "unix",
	}
	s.testConn(c, socketDesc, socketDesc)
}

func (s *SocketSuite) TestTLSOverTCP(c *gc.C) {
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

func (s *SocketSuite) testConn(c *gc.C, serverSocketDesc, clientSocketDesc sockets.Socket) {
	l, err := sockets.Listen(serverSocketDesc)
	c.Assert(err, jc.ErrorIsNil)

	srv := rpc.Server{}
	called := false
	err = srv.Register(RpcCaller(func(arg string, reply *string) error {
		called = true
		*reply = arg
		return nil
	}))
	c.Assert(err, jc.ErrorIsNil)

	go func() {
		cconn, err := sockets.Dial(clientSocketDesc)
		c.Assert(err, jc.ErrorIsNil)
		rep := ""
		err = cconn.Call("RpcCaller.TestCall", "hello", &rep)
		c.Check(err, jc.ErrorIsNil)
		c.Check(rep, gc.Equals, "hello")
		err = cconn.Close()
		c.Assert(err, jc.ErrorIsNil)
	}()

	sconn, err := l.Accept()
	c.Assert(err, jc.ErrorIsNil)
	srv.ServeConn(sconn)
	err = l.Close()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}
