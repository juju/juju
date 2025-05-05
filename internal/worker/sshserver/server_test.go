// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gossh "golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	virtualhostname "github.com/juju/juju/core/virtualhostname"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujutesting "github.com/juju/juju/internal/testing"
)

const maxConcurrentConnections = 10
const testVirtualHostname = "1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local"

type sshServerSuite struct {
	testing.IsolationSuite

	userSigner     ssh.Signer
	sessionHandler *MockSessionHandler
}

var _ = gc.Suite(&sshServerSuite{})

func (s *sshServerSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)

	// Setup user signer
	userKey, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, jc.ErrorIsNil)

	userSigner, err := gossh.NewSignerFromKey(userKey)
	c.Assert(err, jc.ErrorIsNil)

	s.userSigner = userSigner
}

func (s *sshServerSuite) SetUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.sessionHandler = NewMockSessionHandler(ctrl)

	return ctrl
}

func newServerWorkerConfig(
	l logger.Logger,
	j string,
	modifier func(*ServerWorkerConfig),
) *ServerWorkerConfig {
	cfg := &ServerWorkerConfig{
		Logger:               l,
		JumpHostKey:          j,
		NewSSHServerListener: newTestingSSHServerListener,
	}

	modifier(cfg)

	return cfg
}

func (s *sshServerSuite) TestValidate(c *gc.C) {
	cfg := &ServerWorkerConfig{}
	l := loggertesting.WrapCheckLog(c)

	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no Logger.
	cfg = newServerWorkerConfig(l, "Logger", func(cfg *ServerWorkerConfig) {
		cfg.Logger = nil
	})
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no JumpHostKey.
	cfg = newServerWorkerConfig(l, "jumpHostKey", func(cfg *ServerWorkerConfig) {
		cfg.JumpHostKey = ""
	})
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no NewSSHServerListener.
	cfg = newServerWorkerConfig(l, "NewSSHServerListener", func(cfg *ServerWorkerConfig) {
		cfg.NewSSHServerListener = nil
	})
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *sshServerSuite) TestSSHServer(c *gc.C) {
	defer s.SetUpMocks(c).Finish()

	// Firstly, start the server on an in-memory listener
	listener := bufconn.Listen(1024)

	server, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggertesting.WrapCheckLog(c),
		Listener:                 listener,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		NewSSHServerListener:     newTestingSSHServerListener,
		MaxConcurrentConnections: maxConcurrentConnections,
		disableAuth:              true,
		SessionHandler:           s.sessionHandler,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, server)
	workertest.CheckAlive(c, server)

	// Dial the in-memory listener
	conn, err := listener.Dial()
	c.Assert(err, jc.ErrorIsNil)

	// Open a client connection
	jumpConn, chans, terminatingReqs, err := gossh.NewClientConn(
		conn,
		"",
		&gossh.ClientConfig{
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			Auth: []gossh.AuthMethod{
				gossh.Password(""), // No password needed
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Open jump connection
	client := gossh.NewClient(jumpConn, chans, terminatingReqs)
	tunnel, err := client.Dial("tcp", fmt.Sprintf("%s:0", testVirtualHostname))
	c.Assert(err, jc.ErrorIsNil)

	// Now with this opened direct-tcpip channel, open a session connection
	terminatingClientConn, terminatingClientChan, terminatingReqs, err := gossh.NewClientConn(
		tunnel,
		"",
		&gossh.ClientConfig{
			User:            "ubuntu",
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			Auth: []gossh.AuthMethod{
				gossh.PublicKeys(s.userSigner),
			},
		})
	c.Assert(err, jc.ErrorIsNil)

	terminatingClient := gossh.NewClient(terminatingClientConn, terminatingClientChan, terminatingReqs)
	terminatingSession, err := terminatingClient.NewSession()
	c.Assert(err, jc.ErrorIsNil)

	s.sessionHandler.EXPECT().Handle(gomock.Any(), gomock.Any()).DoAndReturn(
		func(session ssh.Session, destination virtualhostname.Info) {
			c.Check(destination.String(), gc.Equals, testVirtualHostname)
			_, _ = session.Write(fmt.Appendf([]byte{}, "Your final destination is: %s\n", destination.String()))
		},
	)
	output, err := terminatingSession.CombinedOutput("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(output), gc.Equals, fmt.Sprintf("Your final destination is: %s\n", testVirtualHostname))

	// Server isn't gracefully closed, it's forcefully closed. All connections ended
	// from server side.
	workertest.CleanKill(c, server)
}

func (s *sshServerSuite) TestSSHServerMaxConnections(c *gc.C) {
	// Firstly, start the server on an in-memory listener
	listener := bufconn.Listen(1024)
	worker, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggertesting.WrapCheckLog(c),
		Listener:                 listener,
		MaxConcurrentConnections: maxConcurrentConnections,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		NewSSHServerListener:     newTestingSSHServerListener,
		disableAuth:              true,
		SessionHandler:           s.sessionHandler,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)

	srv := worker.(*ServerWorker)

	// Check server side that the connection count matches the expected value
	// otherwise we face a race condition in tests where the server hasn't yet
	// decreased the connection count.
	checkConnCount := func(c *gc.C, expected int32) {
		done := time.After(200 * time.Millisecond)
		for {
			connCount := srv.concurrentConnections.Load()
			if connCount == expected {
				return
			}
			select {
			case <-time.After(10 * time.Millisecond):
			case <-done:
				c.Error("timeout waiting for expected connection count")
				return
			}
		}
	}

	// the reason we repeat this test 2 times is to make sure that closing the connections on
	// the first iteration completely resets the counter on the ssh server side.
	for i := range 2 {
		c.Logf("Run %d for TestSSHServerMaxConnections", i)
		clients := make([]*gossh.Client, 0, maxConcurrentConnections)
		config := &gossh.ClientConfig{
			User:            "ubuntu",
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			Auth: []gossh.AuthMethod{
				gossh.PublicKeys(s.userSigner),
			},
		}
		checkConnCount(c, 0)
		for range maxConcurrentConnections {
			client := inMemoryDial(c, listener, config)
			clients = append(clients, client)
		}
		checkConnCount(c, maxConcurrentConnections)
		jumpServerConn, err := listener.Dial()
		c.Assert(err, jc.ErrorIsNil)

		_, _, _, err = gossh.NewClientConn(jumpServerConn, "", config)
		c.Assert(err, gc.ErrorMatches, ".*handshake failed:.*")

		// close the connections
		for _, client := range clients {
			client.Close()
		}
		checkConnCount(c, 0)
		// check the next connection is accepted
		client := inMemoryDial(c, listener, config)
		client.Close()
		checkConnCount(c, 0)
	}
}

// inMemoryDial returns and SSH connection that uses an in-memory transport.
func inMemoryDial(c *gc.C, listener *bufconn.Listener, config *gossh.ClientConfig) *gossh.Client {
	jumpServerConn, err := listener.Dial()
	c.Assert(err, jc.ErrorIsNil)

	sshConn, newChan, reqs, err := gossh.NewClientConn(jumpServerConn, "", config)
	c.Assert(err, jc.ErrorIsNil)
	return gossh.NewClient(sshConn, newChan, reqs)
}

func (s *sshServerSuite) TestSSHWorkerReport(c *gc.C) {
	c.Skip("this test is flaky, skipping until it is fixed")
	defer s.SetUpMocks(c).Finish()

	// Firstly, start the server on an in-memory listener
	listener := bufconn.Listen(1024)
	worker, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggertesting.WrapCheckLog(c),
		Listener:                 listener,
		MaxConcurrentConnections: maxConcurrentConnections,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		NewSSHServerListener:     newTestingSSHServerListener,
		disableAuth:              true,
		SessionHandler:           s.sessionHandler,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)

	report := worker.(*ServerWorker).Report()
	c.Assert(report, gc.DeepEquals, map[string]interface{}{
		"concurrent_connections": int32(0),
	})

	// Dial the in-memory listener
	conn, err := listener.Dial()
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	report = worker.(*ServerWorker).Report()
	c.Assert(report, gc.DeepEquals, map[string]interface{}{
		"concurrent_connections": int32(1),
	})
}
