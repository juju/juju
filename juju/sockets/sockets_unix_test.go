// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sockets_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"regexp"
	"sync"
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

func (s *SocketSuite) TestUNIXSocketPermissions(c *tc.C) {
	socketPath := filepath.Join(c.MkDir(), "a.socket")
	listener, err := sockets.Listen(sockets.Socket{
		Address: socketPath,
		Network: "unix",
	})
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = listener.Close() }()

	info, err := os.Stat(socketPath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Mode().Perm(), tc.Equals, os.FileMode(0700))
}

// TestUNIXSocketKeepsUmask checks that listening on unix sockets from
// several goroutines leaves the process umask alone. The umask is
// inherited by hooks, so a changed one makes charms create files that
// only root can read.
func (s *SocketSuite) TestUNIXSocketKeepsUmask(c *tc.C) {
	socketDir := c.MkDir()
	before := processUmask(c)

	for round := range 100 {
		var wg sync.WaitGroup
		for i := range 4 {
			wg.Go(func() {
				listener, err := sockets.Listen(sockets.Socket{
					Address: filepath.Join(socketDir, fmt.Sprintf("%d-%d.socket", round, i)),
					Network: "unix",
				})
				if c.Check(err, tc.ErrorIsNil) {
					_ = listener.Close()
				}
			})
		}
		wg.Wait()
		c.Assert(processUmask(c), tc.Equals, before)
	}
}

// processUmask reads the umask of the test process without changing it.
func processUmask(c *tc.C) string {
	status, err := os.ReadFile("/proc/self/status")
	c.Assert(err, tc.ErrorIsNil)
	match := regexp.MustCompile(`(?m)^Umask:\s+(\d+)$`).FindSubmatch(status)
	c.Assert(match, tc.HasLen, 2)
	return string(match[1])
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
