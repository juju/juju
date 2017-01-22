// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"net"
	"time"

	_	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/network/ssh"
	coretesting "github.com/juju/juju/testing"
)

type SSHReachableHostPortSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&SSHReachableHostPortSuite{})

func (s *SSHReachableHostPortSuite) TestAllUnreachable(c *gc.C) {
	dialer := &net.Dialer{Timeout: 50 * time.Millisecond}
	unreachableHPs := closedTCPHostPorts(c, 10)
	timeout := 100 * time.Millisecond

	best, err := ssh.ReachableHostPort(unreachableHPs, nil, dialer, timeout)
	c.Check(err, gc.ErrorMatches, "cannot connect to any address: .*")
	c.Check(best, gc.Equals, network.HostPort{})
}

func (s *SSHReachableHostPortSuite) TestReachableInvalidPublicKey(c *gc.C) {
	hostPorts := []network.HostPort{
		testSSHServer(c, "wrong public-key"),
	}
	timeout := 300 * time.Millisecond

	dialer := &net.Dialer{Timeout: 100 * time.Millisecond}
	best, err := ssh.ReachableHostPort(hostPorts, []string{"public-key"}, dialer, timeout)
	c.Check(err, gc.ErrorMatches, "cannot connect to any address: .*")
	c.Check(best, gc.Equals, network.HostPort{})
}

func (s *SSHReachableHostPortSuite) TestReachableValidPublicKey(c *gc.C) {
	hostPorts := []network.HostPort{
		testSSHServer(c, "public-key"),
	}
	timeout := 300 * time.Millisecond

	dialer := &net.Dialer{Timeout: 100 * time.Millisecond}
	best, err := ssh.ReachableHostPort(hostPorts, []string{"public-key"}, dialer, timeout)
	c.Check(err, jc.ErrorIsNil)
	c.Check(best, gc.Equals, hostPorts[0])
}

func (s *SSHReachableHostPortSuite) TestReachableMixedPublicKeys(c *gc.C) {
	// One is just closed, one is TCP only, one is SSH but the wrong key, one
	// is SSH with the right key
	fakeHostPort := closedTCPHostPorts(c, 1)[0]
	hostPorts := []network.HostPort{
		fakeHostPort,
		testTCPServer(c),
		testSSHServer(c, "wrong public-key"),
		testSSHServer(c, "public-key"),
	}
	timeout := 300 * time.Millisecond
	dialer := &net.Dialer{Timeout: 100 * time.Millisecond}
	best, err := ssh.ReachableHostPort(hostPorts, []string{"public-key"}, dialer, timeout)
	c.Check(best, gc.Equals, network.HostPort{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(best, jc.DeepEquals, hostPorts[3])
}

func (s *SSHReachableHostPortSuite) TestReachableNoPublicKeysPassed(c *gc.C) {
	fakeHostPort := closedTCPHostPorts(c, 1)[0]
	hostPorts := []network.HostPort{
		fakeHostPort,
		testTCPServer(c),
	}
	timeout := 300 * time.Millisecond

	dialer := &net.Dialer{Timeout: 100 * time.Millisecond}
	best, err := ssh.ReachableHostPort(hostPorts, nil, dialer, timeout)
	c.Check(err, jc.ErrorIsNil)
	c.Check(best, jc.DeepEquals, hostPorts[1]) // the only real listener
}

func (s *SSHReachableHostPortSuite) TestReachableNoPublicKeysAvailable(c *gc.C) {
	fakeHostPort := closedTCPHostPorts(c, 1)[0]
	hostPorts := []network.HostPort{
		fakeHostPort,
		testTCPServer(c),
	}
	timeout := 300 * time.Millisecond

	dialer := &net.Dialer{Timeout: 100 * time.Millisecond}
	best, err := ssh.ReachableHostPort(hostPorts, []string{"public-key"}, dialer, timeout)
	c.Check(err, gc.ErrorMatches, "cannot connect to any address: .*")
	c.Check(best, gc.Equals, network.HostPort{})
}

const maxTCPPort = 65535

// closedTCPHostPorts opens and then immediately closes a bunch of ports and
// saves their port numbers so we're unlikely to find a real listener at that
// address.
func closedTCPHostPorts(c *gc.C, count int) []network.HostPort {
	ports := make([]network.HostPort, count)
	for i := 0; i < count; i++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		c.Assert(err, jc.ErrorIsNil)
		defer listener.Close()
		listenAddress := listener.Addr().String()
		port, err := network.ParseHostPort(listenAddress)
		c.Assert(err, jc.ErrorIsNil)
		ports[i] = *port
	}
	// By the time we return all the listeners are closed
	return ports
}

// testTCPServer only listens on the socket, but doesn't speak SSH
func testTCPServer(c *gc.C) network.HostPort {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, jc.ErrorIsNil)

	listenAddress := listener.Addr().String()
	hostPort, err := network.ParseHostPort(listenAddress)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("listening on %q", hostPort)

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			c.Logf("accepted connection on %q from %s", hostPort, conn.RemoteAddr())
			conn.Close()
		}
		listener.Close()
	}()

	return *hostPort
}

// testSSHServer will listen on the socket and respond with the appropriate
// public key information and then die. 
func testSSHServer(c *gc.C, publicKey string) network.HostPort {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, jc.ErrorIsNil)

	listenAddress := listener.Addr().String()
	hostPort, err := network.ParseHostPort(listenAddress)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("listening on %q", hostPort)

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			c.Logf("accepted connection on %q from %s", hostPort, conn.RemoteAddr())
			conn.Close()
		}
		listener.Close()
	}()

	return *hostPort
}
