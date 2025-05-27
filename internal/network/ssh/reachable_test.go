// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"crypto/rand"
	"net"
	"testing"
	"time"

	_ "github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/network/ssh"
	sshtesting "github.com/juju/juju/internal/network/ssh/testing"
	coretesting "github.com/juju/juju/internal/testing"
)

type SSHReachableHostPortSuite struct {
	coretesting.BaseSuite
}

func TestSSHReachableHostPortSuite(t *testing.T) {
	tc.Run(t, &SSHReachableHostPortSuite{})
}

var searchTimeout = 300 * time.Millisecond
var dialTimeout = 100 * time.Millisecond

func makeChecker() ssh.ReachableChecker {
	dialer := &net.Dialer{Timeout: dialTimeout}
	checker := ssh.NewReachableChecker(dialer, searchTimeout)
	return checker
}

func (s *SSHReachableHostPortSuite) TestAllUnreachable(c *tc.C) {
	checker := makeChecker()
	unreachableHPs := closedTCPHostPorts(c, 10)
	best, err := checker.FindHost(unreachableHPs, nil)
	c.Check(err, tc.ErrorMatches, "cannot connect to any address: .*")
	c.Check(best, tc.Equals, nil)
}

func (s *SSHReachableHostPortSuite) TestReachableInvalidPublicKey(c *tc.C) {
	hostPorts := network.HostPorts{
		// We use Key2, but are looking for Pub1
		testSSHServer(c, sshtesting.SSHKey2),
	}
	checker := makeChecker()
	best, err := checker.FindHost(hostPorts, []string{sshtesting.SSHPub1})
	c.Check(err, tc.ErrorMatches, "cannot connect to any address: .*")
	c.Check(best, tc.Equals, nil)
}

func (s *SSHReachableHostPortSuite) TestReachableValidPublicKey(c *tc.C) {
	hostPorts := network.HostPorts{
		testSSHServer(c, sshtesting.SSHKey1),
	}
	checker := makeChecker()
	best, err := checker.FindHost(hostPorts, []string{sshtesting.SSHPub1})
	c.Check(err, tc.ErrorIsNil)
	c.Check(best, tc.Equals, hostPorts[0])
}

func (s *SSHReachableHostPortSuite) TestReachableMixedPublicKeys(c *tc.C) {
	// One is just closed, one is TCP only, one is SSH but the wrong key, one
	// is SSH with the right key
	fakeHostPort := closedTCPHostPorts(c, 1)[0]
	hostPorts := network.HostPorts{
		fakeHostPort,
		testTCPServer(c),
		testSSHServer(c, sshtesting.SSHKey2),
		testSSHServer(c, sshtesting.SSHKey1),
	}
	checker := makeChecker()
	best, err := checker.FindHost(hostPorts, []string{sshtesting.SSHPub1})
	c.Check(err, tc.ErrorIsNil)
	c.Check(best, tc.DeepEquals, hostPorts[3])
}

func (s *SSHReachableHostPortSuite) TestReachableNoPublicKeysPassed(c *tc.C) {
	fakeHostPort := closedTCPHostPorts(c, 1)[0]
	hostPorts := network.HostPorts{
		fakeHostPort,
		testTCPServer(c),
		testSSHServer(c, sshtesting.SSHKey1),
	}
	checker := makeChecker()
	// Without a list of public keys, we should just check that the remote host is an SSH server
	best, err := checker.FindHost(hostPorts, nil)
	c.Check(err, tc.ErrorIsNil)
	c.Check(best, tc.DeepEquals, hostPorts[2]) // the only real ssh server
}

func (s *SSHReachableHostPortSuite) TestReachableNoPublicKeysAvailable(c *tc.C) {
	fakeHostPort := closedTCPHostPorts(c, 1)[0]
	hostPorts := network.HostPorts{
		fakeHostPort,
		testTCPServer(c),
	}
	checker := makeChecker()
	best, err := checker.FindHost(hostPorts, []string{sshtesting.SSHPub1})
	c.Check(err, tc.ErrorMatches, "cannot connect to any address: .*")
	c.Check(best, tc.Equals, nil)
}

func (s *SSHReachableHostPortSuite) TestMultiplePublicKeys(c *tc.C) {
	hostPorts := network.HostPorts{
		testSSHServer(c, sshtesting.SSHKey1, sshtesting.SSHKey2),
	}
	checker := makeChecker()
	best, err := checker.FindHost(hostPorts, []string{sshtesting.SSHPub1, sshtesting.SSHPub2})
	c.Check(err, tc.ErrorIsNil)
	c.Check(best, tc.Equals, hostPorts[0])
}

// closedTCPHostPorts opens and then immediately closes a bunch of ports and
// saves their port numbers so we're unlikely to find a real listener at that
// address.
func closedTCPHostPorts(c *tc.C, count int) network.HostPorts {
	randomSuffix := [3]byte{}
	_, _ = rand.Read(randomSuffix[:])
	randomLocal := net.IPv4(127, randomSuffix[0], randomSuffix[1], randomSuffix[2])
	c.Assert(randomLocal.IsLoopback(), tc.IsTrue)
	ports := make(network.MachineHostPorts, count)
	for i := range count {
		listener, err := net.Listen("tcp", net.JoinHostPort(randomLocal.String(), "0"))
		c.Assert(err, tc.ErrorIsNil)
		defer func() { _ = listener.Close() }()
		listenAddress := listener.Addr().String()
		port, err := network.ParseMachineHostPort(listenAddress)
		c.Assert(err, tc.ErrorIsNil)
		ports[i] = *port
	}
	// By the time we return all the listeners are closed
	return ports.HostPorts()
}

// testTCPServer only listens on the socket, but doesn't speak SSH
func testTCPServer(c *tc.C) network.HostPort {
	listenAddress, shutdown := sshtesting.CreateTCPServer(c, func(tcpConn net.Conn) {
		// We accept a connection, but then immediately close.
		_ = tcpConn.Close()
	})
	hostPort, err := network.ParseMachineHostPort(listenAddress)
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { close(shutdown) })

	return *hostPort
}

// testSSHServer will listen on the socket and respond with the appropriate
// public key information and then die.
func testSSHServer(c *tc.C, privateKeys ...string) network.HostPort {
	address, shutdown := sshtesting.CreateSSHServer(c, privateKeys...)
	hostPort, err := network.ParseMachineHostPort(address)
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { close(shutdown) })

	return *hostPort
}
