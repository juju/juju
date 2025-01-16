// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net"
	"time"

	"github.com/juju/testing"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/sshserver"
)

type sshServerSuite struct {
	testing.IsolationSuite

	userSigner  ssh.Signer
	jumpHostKey string
}

var _ = gc.Suite(&sshServerSuite{})

func (s *sshServerSuite) SetUpSuite(c *gc.C) {
	// Setup user signer
	userKey, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, gc.IsNil)

	userSigner, err := ssh.NewSignerFromKey(userKey)
	c.Assert(err, gc.IsNil)

	s.userSigner = userSigner

	// Setup jump host private key
	jumpHostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, gc.IsNil)

	jumpHostKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(jumpHostKey),
	})

	s.jumpHostKey = string(jumpHostKeyPEM)

}

func (s *sshServerSuite) TestSSHServer(c *gc.C) {
	// Firstly, start the server on an in-memory listener
	listener := bufconn.Listen(8 * 1024)
	server, err := sshserver.NewSSHServer(
		nil,
		s.jumpHostKey,
	)
	c.Assert(err, gc.IsNil)

	go func() {
		err := server.Serve(listener)
		c.Assert(err, gc.IsNil)
	}()

	// Dial the in-memory listener
	conn, err := listener.Dial()
	c.Assert(err, gc.IsNil)

	// Open a client connection
	jumpConn, _, terminatingReqs, err := ssh.NewClientConn(
		conn,
		"",
		&ssh.ClientConfig{
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Auth: []ssh.AuthMethod{
				ssh.Password(""), // No password needed
			},
		},
	)
	c.Assert(err, gc.IsNil)
	go ssh.DiscardRequests(terminatingReqs)

	// Open direct-tcpip channel (the -J part of SSH)
	d := struct {
		DestAddr string
		DestPort uint32
		SrcAddr  string
		SrcPort  uint32
	}{
		DestAddr: "1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
		DestPort: 20,
		SrcAddr:  "localhost",
		SrcPort:  0,
	}

	jumpChannel, reqs, err := jumpConn.OpenChannel("direct-tcpip", ssh.Marshal(&d))
	c.Assert(err, gc.IsNil)
	go ssh.DiscardRequests(reqs)

	// Now with this opened direct-tcpip channel, open a session connection
	terminatingClientConn, terminatingClientChan, terminatingReqs, err := ssh.NewClientConn(
		&channelConn{jumpChannel},
		"",
		&ssh.ClientConfig{
			User:            "ubuntu",
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Auth: []ssh.AuthMethod{
				ssh.PublicKeys(s.userSigner),
			},
		})
	c.Assert(err, gc.IsNil)

	terminatingClient := ssh.NewClient(terminatingClientConn, terminatingClientChan, terminatingReqs)
	terminatingSession, err := terminatingClient.NewSession()
	c.Assert(err, gc.IsNil)

	output, err := terminatingSession.CombinedOutput("")
	c.Assert(err, gc.IsNil)
	c.Assert(string(output), gc.Equals, "Your final destination is: 1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local as user: ubuntu\n")
}

type channelConn struct {
	ssh.Channel
}

func (c *channelConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

func (c *channelConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

func (c *channelConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *channelConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *channelConn) SetWriteDeadline(t time.Time) error {
	return nil
}
