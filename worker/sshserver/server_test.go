// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"crypto/rand"
	"crypto/rsa"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/sshserver"
)

type sshServerSuite struct {
	testing.IsolationSuite

	userSigner ssh.Signer
}

var _ = gc.Suite(&sshServerSuite{})

func (s *sshServerSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)

	// Setup user signer
	userKey, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, gc.IsNil)

	userSigner, err := ssh.NewSignerFromKey(userKey)
	c.Assert(err, gc.IsNil)

	s.userSigner = userSigner
}

func (s *sshServerSuite) TestValidate(c *gc.C) {
	cfg := sshserver.ServerWorkerConfig{}
	c.Assert(cfg.Validate(), gc.ErrorMatches, ".*is required.*")
}

func (s *sshServerSuite) TestSSHServer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	l := loggo.GetLogger("test")

	// Firstly, start the server on an in-memory listener
	listener := bufconn.Listen(8 * 1024)

	server, err := sshserver.NewServerWorker(sshserver.ServerWorkerConfig{
		Logger:   l,
		Listener: listener,
	})
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, server)
	workertest.CheckAlive(c, server)

	// Dial the in-memory listener
	conn, err := listener.Dial()
	c.Assert(err, gc.IsNil)

	// Open a client connection
	jumpConn, chans, terminatingReqs, err := ssh.NewClientConn(
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

	// Open jump connection
	client := ssh.NewClient(jumpConn, chans, terminatingReqs)
	tunnel, err := client.Dial("tcp", "1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local:20")
	c.Assert(err, gc.IsNil)

	// Now with this opened direct-tcpip channel, open a session connection
	terminatingClientConn, terminatingClientChan, terminatingReqs, err := ssh.NewClientConn(
		tunnel,
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

	// Server isn't gracefully closed, it's forcefully closed. All connections ended
	// from server side.
	workertest.CleanKill(c, server)
}
