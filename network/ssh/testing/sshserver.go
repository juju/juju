// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"net"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/crypto/ssh"
	gc "gopkg.in/check.v1"
)

// SSHKey1 generated with `ssh-keygen -b 256 -C test-only -t ecdsa -f test-key`
var SSHKey1 = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEILhuaRN6CI4h85SjOFV2+SU1uslRirsyyhGdsVmkKaC2oAoGCCqGSM49
AwEHoUQDQgAESKoQ2r2l3hdXf9K+j+KsTwpTHNWMdd7gsl0tgy+77DYbz7DUDml1
vIBDwimK29kn9WpPU8WSW23ZFPLk53mNTw==
-----END EC PRIVATE KEY-----
`

var SSHPub1 = "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBEiqENq9pd4XV3/Svo/irE8KUxzVjHXe4LJdLYMvu+w2G8+w1A5pdbyAQ8IpitvZJ/VqT1PFkltt2RTy5Od5jU8= test-only"

// SSHKey2 generated with `ssh-keygen -b 256 -C test-only -t ed25519 -f test-key`
var SSHKey2 = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACANk3iR1VrTsEfIyQDXkrZajtOIwmKBdz+hAN90VXdxOQAAAJDQ4EH60OBB
+gAAAAtzc2gtZWQyNTUxOQAAACANk3iR1VrTsEfIyQDXkrZajtOIwmKBdz+hAN90VXdxOQ
AAAEB0Vb6XYd1aFm1dl+37KgqgEeZDuFRlSHjeHrXEDFP4Iw2TeJHVWtOwR8jJANeStlqO
04jCYoF3P6EA33RVd3E5AAAACXRlc3Qtb25seQECAwQ=
-----END OPENSSH PRIVATE KEY-----
`

var SSHPub2 = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIA2TeJHVWtOwR8jJANeStlqO04jCYoF3P6EA33RVd3E5 test-only"

// denyPublicKey implements the SSH PublicKeyCallback API, but just always
// denies any public key it gets.
func denyPublicKey(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	return nil, errors.Errorf("public key denied")
}

// CreateSSHServer launches an SSH server that will use the described private
// key to allow SSH connections. Note that it explicitly doesn't actually
// support any Auth mechanisms, so nobody can complete connections, but it will
// do Key exchange to set up the encrypted conversation.
// We return the address where the SSH service is listening, and a channel
// callers must close when they want the service to stop.
func CreateSSHServer(c *gc.C, privateKeys ...string) (string, chan struct{}) {
	serverConf := &ssh.ServerConfig{
		// We have to set up at least one Auth method, or the SSH server
		// doesn't even try to do key-exchange
		PublicKeyCallback: denyPublicKey,
	}
	for _, privateStr := range privateKeys {
		privateKey, err := ssh.ParsePrivateKey([]byte(privateStr))
		c.Assert(err, jc.ErrorIsNil)
		serverConf.AddHostKey(privateKey)
	}
	// We explicitly listen on IPv4 loopback instead of 'localhost'
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, jc.ErrorIsNil)
	localAddress := listener.Addr().String()

	shutdown := make(chan struct{}, 0)

	go func() {
		for {
			select {
			case <-shutdown:
				// no more listening
				c.Logf("shutting down %s", localAddress)
				listener.Close()
				return
			default:
			}
			// Don't get hung on Accept, set a deadline
			if tcpListener, ok := listener.(*net.TCPListener); ok {
				tcpListener.SetDeadline(time.Now().Add(1 * time.Second))
			}
			tcpConn, err := listener.Accept()
			if err != nil {
				if netErr, ok := err.(net.Error); ok {
					if netErr.Timeout() {
						// Try again, so we reevaluate if we need to shut down
						continue
					}
				}
			}
			if err != nil {
				c.Logf("failed to accept connection on %s: %v", localAddress, err)
				continue
			}
			remoteAddress := tcpConn.RemoteAddr().String()
			c.Logf("accepted tcp connection for ssh on %s from %s", localAddress, remoteAddress)
			sshConn, _, _, err := ssh.NewServerConn(tcpConn, serverConf)
			if err != nil {
				// TODO: some errors are expected, as we don't support Auth
				c.Logf("error initiating ssh connection for %s: %v", remoteAddress, err)
			} else {
				// We don't expect to get here, but if we do, make sure we close the connection.
				sshConn.Close()
			}
		}
	}()
	return localAddress, shutdown
}
