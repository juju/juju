// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"crypto/rand"
	"crypto/rsa"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/sshserver"
)

const maxConcurrentConnections = 10

type sshServerSuite struct {
	testing.IsolationSuite

	userSigner ssh.Signer
}

var _ = gc.Suite(&sshServerSuite{})

func (s *sshServerSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)

	// Setup user signer
	userKey, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, jc.ErrorIsNil)

	userSigner, err := ssh.NewSignerFromKey(userKey)
	c.Assert(err, jc.ErrorIsNil)

	s.userSigner = userSigner
}

func newServerWorkerConfig(
	l logger.Logger,
	j string,
	modifier func(*sshserver.ServerWorkerConfig),
) *sshserver.ServerWorkerConfig {
	cfg := &sshserver.ServerWorkerConfig{
		Logger:               l,
		JumpHostKey:          j,
		NewSSHServerListener: newTestingSSHServerListener,
	}

	modifier(cfg)

	return cfg
}

func (s *sshServerSuite) TestValidate(c *gc.C) {
	cfg := &sshserver.ServerWorkerConfig{}
	l := loggertesting.WrapCheckLog(c)

	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Test no Logger.
	cfg = newServerWorkerConfig(l, "Logger", func(cfg *sshserver.ServerWorkerConfig) {
		cfg.Logger = nil
	})
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no JumpHostKey.
	cfg = newServerWorkerConfig(l, "jumpHostKey", func(cfg *sshserver.ServerWorkerConfig) {
		cfg.JumpHostKey = ""
	})
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no NewSSHServerListener.
	cfg = newServerWorkerConfig(l, "NewSSHServerListener", func(cfg *sshserver.ServerWorkerConfig) {
		cfg.NewSSHServerListener = nil
	})
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *sshServerSuite) TestSSHServer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Firstly, start the server on an in-memory listener
	listener := bufconn.Listen(8 * 1024)

	server, err := sshserver.NewServerWorker(sshserver.ServerWorkerConfig{
		Logger:                   loggertesting.WrapCheckLog(c),
		Listener:                 listener,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		NewSSHServerListener:     newTestingSSHServerListener,
		MaxConcurrentConnections: maxConcurrentConnections,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, server)
	workertest.CheckAlive(c, server)

	// Dial the in-memory listener
	conn, err := listener.Dial()
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)

	// Open jump connection
	client := ssh.NewClient(jumpConn, chans, terminatingReqs)
	tunnel, err := client.Dial("tcp", "1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local:20")
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)

	terminatingClient := ssh.NewClient(terminatingClientConn, terminatingClientChan, terminatingReqs)
	terminatingSession, err := terminatingClient.NewSession()
	c.Assert(err, jc.ErrorIsNil)

	output, err := terminatingSession.CombinedOutput("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(output), gc.Equals, "Your final destination is: 1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local as user: ubuntu\n")

	// Server isn't gracefully closed, it's forcefully closed. All connections ended
	// from server side.
	workertest.CleanKill(c, server)
}

func (s *sshServerSuite) TestSSHServerMaxConnections(c *gc.C) {
	// Firstly, start the server on an in-memory listener
	listener := bufconn.Listen(8 * 1024)
	worker, err := sshserver.NewServerWorker(sshserver.ServerWorkerConfig{
		Logger:                   loggertesting.WrapCheckLog(c),
		Listener:                 listener,
		MaxConcurrentConnections: maxConcurrentConnections,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		NewSSHServerListener:     newTestingSSHServerListener,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	// the reason we repeat this test 2 times is to make sure that closing the connections on
	// the first iteration completely resets the counter on the ssh server side.
	for i := range 2 {
		c.Logf("Run %d for TestSSHServerMaxConnections", i)
		clients := make([]*ssh.Client, 0, maxConcurrentConnections)
		config := &ssh.ClientConfig{
			User:            "ubuntu",
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Auth: []ssh.AuthMethod{
				ssh.PublicKeys(s.userSigner),
			},
		}
		for range maxConcurrentConnections {
			client := inMemoryDial(c, listener, config)
			_, err := client.Dial("tcp", "1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local:20")
			c.Assert(err, jc.ErrorIsNil)
			clients = append(clients, client)
		}
		jumpServerConn, err := listener.Dial()
		c.Assert(err, jc.ErrorIsNil)

		_, _, _, err = ssh.NewClientConn(jumpServerConn, "", config)
		c.Assert(err, gc.ErrorMatches, ".*handshake failed: EOF.*")

		// close the connections
		for _, client := range clients {
			client.Close()
		}
		// check the next connection is accepted
		client := inMemoryDial(c, listener, config)
		client.Close()
	}
}

// inMemoryDial returns and SSH connection that uses an in-memory transport.
func inMemoryDial(c *gc.C, listener *bufconn.Listener, config *ssh.ClientConfig) *ssh.Client {
	jumpServerConn, err := listener.Dial()
	c.Assert(err, jc.ErrorIsNil)

	sshConn, newChan, reqs, err := ssh.NewClientConn(jumpServerConn, "", config)
	c.Assert(err, jc.ErrorIsNil)
	return ssh.NewClient(sshConn, newChan, reqs)
}

func (s *sshServerSuite) TestSSHWorkerReport(c *gc.C) {
	// Firstly, start the server on an in-memory listener
	listener := bufconn.Listen(8 * 1024)
	worker, err := sshserver.NewServerWorker(sshserver.ServerWorkerConfig{
		Logger:                   loggertesting.WrapCheckLog(c),
		Listener:                 listener,
		MaxConcurrentConnections: maxConcurrentConnections,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		NewSSHServerListener:     newTestingSSHServerListener,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)

	report := worker.(*sshserver.ServerWorker).Report()
	c.Assert(report, gc.DeepEquals, map[string]interface{}{
		"concurrent_connections": int32(0),
	})

	// Dial the in-memory listener
	conn, err := listener.Dial()
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	report = worker.(*sshserver.ServerWorker).Report()
	c.Assert(report, gc.DeepEquals, map[string]interface{}{
		"concurrent_connections": int32(1),
	})
}
