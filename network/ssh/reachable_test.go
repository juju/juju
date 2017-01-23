// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"net"
	"time"

	_ "github.com/juju/errors"
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

// sshKey1 generated with `ssh-keygen -b 256 -C test-only -t ecdsa -f test-key`
var sshKey1 = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEILhuaRN6CI4h85SjOFV2+SU1uslRirsyyhGdsVmkKaC2oAoGCCqGSM49
AwEHoUQDQgAESKoQ2r2l3hdXf9K+j+KsTwpTHNWMdd7gsl0tgy+77DYbz7DUDml1
vIBDwimK29kn9WpPU8WSW23ZFPLk53mNTw==
-----END EC PRIVATE KEY-----
`

var sshPub1 = "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBEiqENq9pd4XV3/Svo/irE8KUxzVjHXe4LJdLYMvu+w2G8+w1A5pdbyAQ8IpitvZJ/VqT1PFkltt2RTy5Od5jU8= test-only"

// sshKey2 generated with `ssh-keygen -b 256 -C test-only -t ed25519 -f test-key`
var sshKey2 = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACANk3iR1VrTsEfIyQDXkrZajtOIwmKBdz+hAN90VXdxOQAAAJDQ4EH60OBB
+gAAAAtzc2gtZWQyNTUxOQAAACANk3iR1VrTsEfIyQDXkrZajtOIwmKBdz+hAN90VXdxOQ
AAAEB0Vb6XYd1aFm1dl+37KgqgEeZDuFRlSHjeHrXEDFP4Iw2TeJHVWtOwR8jJANeStlqO
04jCYoF3P6EA33RVd3E5AAAACXRlc3Qtb25seQECAwQ=
-----END OPENSSH PRIVATE KEY-----
`

var sshPub2 = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIA2TeJHVWtOwR8jJANeStlqO04jCYoF3P6EA33RVd3E5 test-only"

var searchTimeout = 300 * time.Millisecond
var dialTimeout = 100 * time.Millisecond

func (s *SSHReachableHostPortSuite) TestAllUnreachable(c *gc.C) {
	dialer := &net.Dialer{Timeout: dialTimeout}
	unreachableHPs := closedTCPHostPorts(c, 10)
	best, err := ssh.ReachableHostPort(unreachableHPs, nil, dialer, searchTimeout)
	c.Check(err, gc.ErrorMatches, "cannot connect to any address: .*")
	c.Check(best, gc.Equals, network.HostPort{})
}

func (s *SSHReachableHostPortSuite) TestReachableInvalidPublicKey(c *gc.C) {
	hostPorts := []network.HostPort{
		// We use Key2, but are looking for Pub1
		testSSHServer(c, sshKey2),
	}
	dialer := &net.Dialer{Timeout: dialTimeout}
	best, err := ssh.ReachableHostPort(hostPorts, []string{sshPub1}, dialer, searchTimeout)
	c.Check(err, gc.ErrorMatches, "cannot connect to any address: .*")
	c.Check(best, gc.Equals, network.HostPort{})
}

func (s *SSHReachableHostPortSuite) TestReachableValidPublicKey(c *gc.C) {
	hostPorts := []network.HostPort{
		testSSHServer(c, sshKey1),
	}
	dialer := &net.Dialer{Timeout: dialTimeout}
	best, err := ssh.ReachableHostPort(hostPorts, []string{sshPub1}, dialer, searchTimeout)
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
		testSSHServer(c, sshKey2),
		testSSHServer(c, sshKey1),
	}
	dialer := &net.Dialer{Timeout: dialTimeout}
	best, err := ssh.ReachableHostPort(hostPorts, []string{sshPub1}, dialer, searchTimeout)
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
	dialer := &net.Dialer{Timeout: dialTimeout}
	best, err := ssh.ReachableHostPort(hostPorts, nil, dialer, searchTimeout)
	c.Check(err, jc.ErrorIsNil)
	c.Check(best, jc.DeepEquals, hostPorts[1]) // the only real listener
}

func (s *SSHReachableHostPortSuite) TestReachableNoPublicKeysAvailable(c *gc.C) {
	fakeHostPort := closedTCPHostPorts(c, 1)[0]
	hostPorts := []network.HostPort{
		fakeHostPort,
		testTCPServer(c),
	}
	dialer := &net.Dialer{Timeout: dialTimeout}
	best, err := ssh.ReachableHostPort(hostPorts, []string{sshPub1}, dialer, searchTimeout)
	c.Check(err, gc.ErrorMatches, "cannot connect to any address: .*")
	c.Check(best, gc.Equals, network.HostPort{})
}

func (s *SSHReachableHostPortSuite) TestMultiplePublicKeys(c *gc.C) {
	hostPorts := []network.HostPort{
		testSSHServer(c, sshKey2),
	}
	dialer := &net.Dialer{Timeout: dialTimeout}
	best, err := ssh.ReachableHostPort(hostPorts, []string{sshPub1, sshPub2}, dialer, searchTimeout)
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
