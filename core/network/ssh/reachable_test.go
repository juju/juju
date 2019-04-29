// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"net"
	"time"

	_ "github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/ssh"
	sshtesting "github.com/juju/juju/core/network/ssh/testing"
	coretesting "github.com/juju/juju/testing"
)

type SSHReachableHostPortSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&SSHReachableHostPortSuite{})

var searchTimeout = 300 * time.Millisecond
var dialTimeout = 100 * time.Millisecond

func makeChecker() ssh.ReachableChecker {
	dialer := &net.Dialer{Timeout: dialTimeout}
	checker := ssh.NewReachableChecker(dialer, searchTimeout)
	return checker
}

func (s *SSHReachableHostPortSuite) TestAllUnreachable(c *gc.C) {
	checker := makeChecker()
	unreachableHPs := closedTCPHostPorts(c, 10)
	best, err := checker.FindHost(unreachableHPs, nil)
	c.Check(err, gc.ErrorMatches, "cannot connect to any address: .*")
	c.Check(best, gc.Equals, network.HostPort{})
}

func (s *SSHReachableHostPortSuite) TestReachableInvalidPublicKey(c *gc.C) {
	hostPorts := []network.HostPort{
		// We use Key2, but are looking for Pub1
		testSSHServer(c, s, sshtesting.SSHKey2),
	}
	checker := makeChecker()
	best, err := checker.FindHost(hostPorts, []string{sshtesting.SSHPub1})
	c.Check(err, gc.ErrorMatches, "cannot connect to any address: .*")
	c.Check(best, gc.Equals, network.HostPort{})
}

func (s *SSHReachableHostPortSuite) TestReachableValidPublicKey(c *gc.C) {
	hostPorts := []network.HostPort{
		testSSHServer(c, s, sshtesting.SSHKey1),
	}
	checker := makeChecker()
	best, err := checker.FindHost(hostPorts, []string{sshtesting.SSHPub1})
	c.Check(err, jc.ErrorIsNil)
	c.Check(best, gc.Equals, hostPorts[0])
}

func (s *SSHReachableHostPortSuite) TestReachableMixedPublicKeys(c *gc.C) {
	// One is just closed, one is TCP only, one is SSH but the wrong key, one
	// is SSH with the right key
	fakeHostPort := closedTCPHostPorts(c, 1)[0]
	hostPorts := []network.HostPort{
		fakeHostPort,
		testTCPServer(c, s),
		testSSHServer(c, s, sshtesting.SSHKey2),
		testSSHServer(c, s, sshtesting.SSHKey1),
	}
	checker := makeChecker()
	best, err := checker.FindHost(hostPorts, []string{sshtesting.SSHPub1})
	c.Check(err, jc.ErrorIsNil)
	c.Check(best, jc.DeepEquals, hostPorts[3])
}

func (s *SSHReachableHostPortSuite) TestReachableNoPublicKeysPassed(c *gc.C) {
	fakeHostPort := closedTCPHostPorts(c, 1)[0]
	hostPorts := []network.HostPort{
		fakeHostPort,
		testTCPServer(c, s),
		testSSHServer(c, s, sshtesting.SSHKey1),
	}
	checker := makeChecker()
	// Without a list of public keys, we should just check that the remote host is an SSH server
	best, err := checker.FindHost(hostPorts, nil)
	c.Check(err, jc.ErrorIsNil)
	c.Check(best, jc.DeepEquals, hostPorts[2]) // the only real ssh server
}

func (s *SSHReachableHostPortSuite) TestReachableNoPublicKeysAvailable(c *gc.C) {
	fakeHostPort := closedTCPHostPorts(c, 1)[0]
	hostPorts := []network.HostPort{
		fakeHostPort,
		testTCPServer(c, s),
	}
	checker := makeChecker()
	best, err := checker.FindHost(hostPorts, []string{sshtesting.SSHPub1})
	c.Check(err, gc.ErrorMatches, "cannot connect to any address: .*")
	c.Check(best, gc.Equals, network.HostPort{})
}

func (s *SSHReachableHostPortSuite) TestMultiplePublicKeys(c *gc.C) {
	hostPorts := []network.HostPort{
		testSSHServer(c, s, sshtesting.SSHKey1, sshtesting.SSHKey2),
	}
	checker := makeChecker()
	best, err := checker.FindHost(hostPorts, []string{sshtesting.SSHPub1, sshtesting.SSHPub2})
	c.Check(err, jc.ErrorIsNil)
	c.Check(best, gc.Equals, hostPorts[0])
}

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

type Cleaner interface {
	AddCleanup(cleanup func(*gc.C))
}

// testTCPServer only listens on the socket, but doesn't speak SSH
func testTCPServer(c *gc.C, cleaner Cleaner) network.HostPort {
	listenAddress, shutdown := sshtesting.CreateTCPServer(c, func(tcpConn net.Conn) {
		// We accept a connection, but then immediately close.
		tcpConn.Close()
	})
	hostPort, err := network.ParseHostPort(listenAddress)
	c.Assert(err, jc.ErrorIsNil)
	cleaner.AddCleanup(func(*gc.C) { close(shutdown) })

	return *hostPort
}

// testSSHServer will listen on the socket and respond with the appropriate
// public key information and then die.
func testSSHServer(c *gc.C, cleaner Cleaner, privateKeys ...string) network.HostPort {
	address, shutdown := sshtesting.CreateSSHServer(c, privateKeys...)
	hostPort, err := network.ParseHostPort(address)
	c.Assert(err, jc.ErrorIsNil)
	cleaner.AddCleanup(func(*gc.C) { close(shutdown) })

	return *hostPort
}
