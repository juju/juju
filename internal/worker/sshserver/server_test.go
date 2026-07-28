// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"fmt"
	net "net"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki/test"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

const maxConcurrentConnections = 10
const testVirtualHostname = "1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local"

type sshServerSuite struct {
	testhelpers.IsolationSuite

	userSigner    ssh.Signer
	authenticator *MockAuthenticator
	authorizer    *MockAuthorizer
	proxyFactory  *MockProxyFactory
	proxyHandlers *MockProxyHandlers
	tunnelTracker *MockTunnelTracker
}

func TestSshServerSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &sshServerSuite{})
	})
}

func (s *sshServerSuite) SetUpSuite(c *tc.C) {
	s.IsolationSuite.SetUpSuite(c)

	// Setup user signer
	privateKey, err := test.InsecureKeyProfile()
	c.Assert(err, tc.ErrorIsNil)

	signer, err := gossh.NewSignerFromSigner(privateKey)
	c.Assert(err, tc.ErrorIsNil)

	s.userSigner = signer
}

func (s *sshServerSuite) SetUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.authenticator = NewMockAuthenticator(ctrl)
	s.authorizer = NewMockAuthorizer(ctrl)
	s.proxyFactory = NewMockProxyFactory(ctrl)
	s.proxyHandlers = NewMockProxyHandlers(ctrl)
	s.tunnelTracker = NewMockTunnelTracker(ctrl)

	s.authenticator.EXPECT().PublicKeyAuthentication(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	s.authenticator.EXPECT().PasswordAuthentication(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	s.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	s.proxyFactory.EXPECT().New(gomock.Any()).Return(s.proxyHandlers, nil).AnyTimes()
	s.proxyHandlers.EXPECT().DirectTCPIPHandler().Return(rejectDirectTCPIP).AnyTimes()
	s.proxyHandlers.EXPECT().SFTPHandler().Return(rejectSFTP).AnyTimes()
	s.tunnelTracker.EXPECT().AuthenticateTunnel(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	s.tunnelTracker.EXPECT().PushTunnel(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	c.Cleanup(func() {
		s.authenticator = nil
		s.authorizer = nil
		s.proxyFactory = nil
		s.proxyHandlers = nil
		s.tunnelTracker = nil
	})
	return ctrl
}

func newServerWorkerConfig(
	l logger.Logger,
	j string,
	modifier func(*ServerWorkerConfig),
) *ServerWorkerConfig {
	cfg := &ServerWorkerConfig{
		Logger:      l,
		JumpHostKey: j,
		SSHService:  stubSSHService{jumpHostKey: testHostKey, virtualHostKey: testHostKey},
	}

	modifier(cfg)

	return cfg
}

func (s *sshServerSuite) TestValidate(c *tc.C) {
	cfg := &ServerWorkerConfig{}
	l := loggertesting.WrapCheckLog(c)

	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	// Test no Logger.
	cfg = newServerWorkerConfig(l, "Logger", func(cfg *ServerWorkerConfig) {
		cfg.Logger = nil
	})
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	// Test no JumpHostKey.
	cfg = newServerWorkerConfig(l, "jumpHostKey", func(cfg *ServerWorkerConfig) {
		cfg.JumpHostKey = ""
	})
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	// Test no SSHService.
	cfg = newServerWorkerConfig(l, "jumpHostKey", func(cfg *ServerWorkerConfig) {
		cfg.SSHService = nil
	})
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *sshServerSuite) TestSSHServer(c *tc.C) {
	defer s.SetUpMocks(c).Finish()

	// Start a real unix domain socket at a random name.
	endpoint := "@" + uuid.MustNewUUID().String()
	listener, err := net.Listen("unix", endpoint)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = listener.Close() }()

	server, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggertesting.WrapCheckLog(c),
		Listener:                 listener,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		SSHService:               stubSSHService{jumpHostKey: testHostKey, virtualHostKey: jujutesting.SSHServerHostKey},
		MaxConcurrentConnections: maxConcurrentConnections,
		Authenticator:            s.authenticator,
		Authorizer:               s.authorizer,
		ProxyFactory:             s.proxyFactory,
		TunnelTracker:            s.tunnelTracker,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, server)
	workertest.CheckAlive(c, server)

	// Dial the in-memory listener
	conn, err := net.Dial("unix", endpoint)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = conn.Close() }()

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
	c.Assert(err, tc.ErrorIsNil)

	// Open jump connection
	client := gossh.NewClient(jumpConn, chans, terminatingReqs)
	tunnel, err := client.Dial("tcp", fmt.Sprintf("%s:0", testVirtualHostname))
	c.Assert(err, tc.ErrorIsNil)

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
	c.Assert(err, tc.ErrorIsNil)

	terminatingClient := gossh.NewClient(terminatingClientConn, terminatingClientChan, terminatingReqs)
	terminatingSession, err := terminatingClient.NewSession()
	c.Assert(err, tc.ErrorIsNil)

	s.proxyHandlers.EXPECT().SessionHandler(gomock.Any()).Do(func(session ssh.Session) {
		_, _ = session.Write(fmt.Appendf([]byte{}, "Your final destination is: %s\n", testVirtualHostname))
	})
	output, err := terminatingSession.CombinedOutput("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(output), tc.Equals, fmt.Sprintf("Your final destination is: %s\n", testVirtualHostname))

	// Server isn't gracefully closed, it's forcefully closed. All connections ended
	// from server side.
	workertest.CleanKill(c, server)
}

func (s *sshServerSuite) TestSSHServerMaxConnections(c *tc.C) {
	defer s.SetUpMocks(c).Finish()

	// Start a real unix domain socket at a random name.
	endpoint := "@" + uuid.MustNewUUID().String()
	listener, err := net.Listen("unix", endpoint)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = listener.Close() }()

	worker, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggertesting.WrapCheckLog(c),
		Listener:                 listener,
		MaxConcurrentConnections: maxConcurrentConnections,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		SSHService:               stubSSHService{jumpHostKey: testHostKey, virtualHostKey: testHostKey},
		Authenticator:            s.authenticator,
		Authorizer:               s.authorizer,
		ProxyFactory:             s.proxyFactory,
		TunnelTracker:            s.tunnelTracker,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)

	srv := worker.(*ServerWorker)

	// Check server side that the connection count matches the expected value
	// otherwise we face a race condition in tests where the server hasn't yet
	// decreased the connection count.
	checkConnCount := func(c *tc.C, expected int32) {
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
			client := dial(c, "unix", endpoint, config)
			clients = append(clients, client)
		}
		checkConnCount(c, maxConcurrentConnections)
		jumpServerConn, err := net.Dial("unix", endpoint)
		c.Assert(err, tc.ErrorIsNil)

		_, _, _, err = gossh.NewClientConn(jumpServerConn, "", config)
		c.Assert(err, tc.ErrorMatches, ".*handshake failed:.*")

		// close the connections
		for _, client := range clients {
			client.Close()
		}
		checkConnCount(c, 0)
		// check the next connection is accepted
		client := dial(c, "unix", endpoint, config)
		client.Close()
		checkConnCount(c, 0)
	}
}

// dial returns and SSH connection that uses an in-memory transport.
func dial(c *tc.C, network string, addr string, config *gossh.ClientConfig) *gossh.Client {
	jumpServerConn, err := net.Dial(network, addr)
	c.Assert(err, tc.ErrorIsNil)

	sshConn, newChan, reqs, err := gossh.NewClientConn(jumpServerConn, "", config)
	c.Assert(err, tc.ErrorIsNil)
	return gossh.NewClient(sshConn, newChan, reqs)
}

func (s *sshServerSuite) TestSSHWorkerReport(c *tc.C) {
	defer s.SetUpMocks(c).Finish()

	// Start a real unix domain socket at a random name.
	endpoint := "@" + uuid.MustNewUUID().String()
	listener, err := net.Listen("unix", endpoint)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = listener.Close() }()

	worker, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggertesting.WrapCheckLog(c),
		Listener:                 listener,
		MaxConcurrentConnections: maxConcurrentConnections,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		SSHService:               stubSSHService{jumpHostKey: testHostKey, virtualHostKey: testHostKey},
		Authenticator:            s.authenticator,
		Authorizer:               s.authorizer,
		ProxyFactory:             s.proxyFactory,
		TunnelTracker:            s.tunnelTracker,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)

	report := worker.(*ServerWorker).Report(c.Context())
	c.Assert(report, tc.DeepEquals, map[string]any{
		"concurrent_connections": int32(0),
	})

	// Dial the listener
	config := &gossh.ClientConfig{
		User:            "ubuntu",
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(s.userSigner),
		},
	}
	client := dial(c, "unix", endpoint, config)
	defer func() { _ = client.Close() }()

	report = worker.(*ServerWorker).Report(c.Context())
	c.Assert(report, tc.DeepEquals, map[string]any{
		"concurrent_connections": int32(1),
	})
}

func rejectDirectTCPIP(_ *ssh.Server, _ *gossh.ServerConn, newChan gossh.NewChannel, _ ssh.Context) {
	_ = newChan.Reject(gossh.Prohibited, "not implemented")
}

func rejectSFTP(session ssh.Session) {
	_, _ = session.Stderr().Write([]byte("not implemented\n"))
	_ = session.Exit(1)
}
