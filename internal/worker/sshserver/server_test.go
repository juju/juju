// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	gossh "golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"

	"github.com/juju/juju/core/logger"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/virtualhostname"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki/test"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
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
	return ctrl
}

func (s *sshServerSuite) newServer(c *tc.C) (*ServerWorker, *bufconn.Listener, func()) {
	listener := bufconn.Listen(1024)

	cfg := ServerWorkerConfig{
		Logger:                   loggertesting.WrapCheckLog(c),
		Listener:                 listener,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		SSHService:               stubSSHService{jumpHostKey: testHostKey, virtualHostKey: jujutesting.SSHServerHostKey},
		MaxConcurrentConnections: maxConcurrentConnections,
		Authenticator:            s.authenticator,
		Authorizer:               s.authorizer,
		ProxyFactory:             s.proxyFactory,
		TunnelTracker:            s.tunnelTracker,
	}

	worker, err := NewServerWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	server := worker.(*ServerWorker)
	cleanup := func() {
		workertest.CleanKill(c, server)
	}
	workertest.CheckAlive(c, server)
	return server, listener, cleanup
}

func dialSSHServer(c *tc.C, listener *bufconn.Listener, user string, auth gossh.AuthMethod) *gossh.Client {
	conn, err := listener.Dial()
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { _ = conn.Close() })

	sshConn, chans, reqs, err := gossh.NewClientConn(conn, "", &gossh.ClientConfig{
		User:            user,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Auth:            []gossh.AuthMethod{auth},
	})
	c.Assert(err, tc.ErrorIsNil)

	client := gossh.NewClient(sshConn, chans, reqs)
	c.Cleanup(func() { _ = client.Close() })
	return client
}

func dialSSHServerWithError(c *tc.C, listener *bufconn.Listener, user string, auth gossh.AuthMethod) error {
	conn, err := listener.Dial()
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { _ = conn.Close() })

	_, _, _, err = gossh.NewClientConn(conn, "", &gossh.ClientConfig{
		User:            user,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Auth:            []gossh.AuthMethod{auth},
	})
	return err
}

func (s *sshServerSuite) openTerminatingSession(c *tc.C, client *gossh.Client, destination string) *gossh.Session {
	tunnel, err := client.Dial("tcp", fmt.Sprintf("%s:0", destination))
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { _ = tunnel.Close() })

	terminatingConn, chans, reqs, err := gossh.NewClientConn(
		tunnel,
		"",
		&gossh.ClientConfig{
			User:            "ubuntu",
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			Auth:            []gossh.AuthMethod{gossh.PublicKeys(s.userSigner)},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	terminatingClient := gossh.NewClient(terminatingConn, chans, reqs)
	c.Cleanup(func() { _ = terminatingClient.Close() })
	terminatingSession, err := terminatingClient.NewSession()
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { _ = terminatingSession.Close() })
	return terminatingSession
}

func (s *sshServerSuite) testSSHServerSession(c *tc.C, auth gossh.AuthMethod, username string) {
	destination, err := virtualhostname.Parse(testVirtualHostname)
	c.Assert(err, tc.ErrorIsNil)

	// Authorize the user and setup the proxy factory and handlers.
	s.authorizer.EXPECT().Authorize(gomock.Any(), destination).Return(true, nil)
	s.proxyFactory.EXPECT().New(destination).Return(s.proxyHandlers, nil)
	s.proxyHandlers.EXPECT().DirectTCPIPHandler().Return(rejectDirectTCPIP)
	s.proxyHandlers.EXPECT().SFTPHandler().Return(rejectSFTP)

	sessionOutput := fmt.Sprintf("Your final destination is: %s\n", testVirtualHostname)
	s.proxyHandlers.EXPECT().SessionHandler(gomock.Any()).Do(func(session ssh.Session) {
		_, _ = session.Write([]byte(sessionOutput))
	})

	_, listener, cleanup := s.newServer(c)
	defer cleanup()
	client := dialSSHServer(c, listener, username, auth)
	terminatingSession := s.openTerminatingSession(c, client, testVirtualHostname)

	var output bytes.Buffer
	terminatingSession.Stdout = &output
	err = terminatingSession.Run("")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(output.String(), tc.Equals, fmt.Sprintf("Your final destination is: %s\n", testVirtualHostname))
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

	// Test no Authenticator.
	cfg = newServerWorkerConfig(l, "jumpHostKey", func(cfg *ServerWorkerConfig) {
		cfg.Authenticator = nil
	})
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	// Test no Authorizer.
	cfg = newServerWorkerConfig(l, "jumpHostKey", func(cfg *ServerWorkerConfig) {
		cfg.Authorizer = nil
	})
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	// Test no ProxyFactory.
	cfg = newServerWorkerConfig(l, "jumpHostKey", func(cfg *ServerWorkerConfig) {
		cfg.ProxyFactory = nil
	})
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	// Test no TunnelTracker.
	cfg = newServerWorkerConfig(l, "jumpHostKey", func(cfg *ServerWorkerConfig) {
		cfg.TunnelTracker = nil
	})
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *sshServerSuite) TestSSHServerSession(c *tc.C) {
	s.SetUpMocks(c)

	// Test password authentication.
	s.authenticator.EXPECT().PasswordAuthentication(gomock.Any(), "test-password").Return(true, nil)
	s.testSSHServerSession(c, gossh.Password("test-password"), "test-user")

	// Test public key authentication.
	s.authenticator.EXPECT().PublicKeyAuthentication(gomock.Any(), s.userSigner.PublicKey()).Return(true, nil)
	s.testSSHServerSession(c, gossh.PublicKeys(s.userSigner), "test-user")
}

func (s *sshServerSuite) TestJumpServerAuthenticationForbidden(c *tc.C) {
	s.SetUpMocks(c)

	s.authenticator.EXPECT().PublicKeyAuthentication(gomock.Any(), s.userSigner.PublicKey()).Return(false, nil)
	s.authenticator.EXPECT().PasswordAuthentication(gomock.Any(), "password").Return(false, nil)

	_, listener, cleanup := s.newServer(c)
	defer cleanup()
	err := dialSSHServerWithError(c, listener, "alice", gossh.PublicKeys(s.userSigner))
	c.Check(err, tc.ErrorMatches, ".*unable to authenticate.*")
	err = dialSSHServerWithError(c, listener, "alice", gossh.Password("password"))
	c.Check(err, tc.ErrorMatches, ".*unable to authenticate.*")
}

func (s *sshServerSuite) TestJumpServerAuthenticationRejectErrors(c *tc.C) {
	s.SetUpMocks(c)

	s.authenticator.EXPECT().PublicKeyAuthentication(gomock.Any(), s.userSigner.PublicKey()).Return(false, errors.New("invalid key"))
	s.authenticator.EXPECT().PasswordAuthentication(gomock.Any(), "password").Return(false, errors.New("invalid password"))

	_, listener, cleanup := s.newServer(c)
	defer cleanup()
	err := dialSSHServerWithError(c, listener, "alice", gossh.PublicKeys(s.userSigner))
	c.Check(err, tc.ErrorMatches, ".*unable to authenticate.*")
	err = dialSSHServerWithError(c, listener, "alice", gossh.Password("password"))
	c.Check(err, tc.ErrorMatches, ".*unable to authenticate.*")
}

func (s *sshServerSuite) TestTerminatingSSHServerReportsFactoryError(c *tc.C) {
	s.SetUpMocks(c)

	destination, err := virtualhostname.Parse(testVirtualHostname)
	c.Assert(err, tc.ErrorIsNil)
	s.authenticator.EXPECT().PublicKeyAuthentication(gomock.Any(), s.userSigner.PublicKey()).Return(true, nil)
	s.authorizer.EXPECT().Authorize(gomock.Any(), destination).Return(true, nil)
	s.proxyFactory.EXPECT().New(destination).Return(nil, errors.New("factory failed"))

	_, listener, cleanup := s.newServer(c)
	defer cleanup()
	client := dialSSHServer(c, listener, "alice", gossh.PublicKeys(s.userSigner))
	_, err = client.Dial("tcp", fmt.Sprintf("%s:0", testVirtualHostname))
	c.Check(err, tc.ErrorMatches, ".*failed to create embedded server: factory failed.*")
}

func (s *sshServerSuite) TestServerRejectsUnauthorizedDestination(c *tc.C) {
	s.SetUpMocks(c)

	destination, err := virtualhostname.Parse(testVirtualHostname)
	c.Assert(err, tc.ErrorIsNil)
	s.authenticator.EXPECT().PublicKeyAuthentication(gomock.Any(), s.userSigner.PublicKey()).Return(true, nil)
	s.authorizer.EXPECT().Authorize(gomock.Any(), destination).Return(false, nil)

	_, listener, cleanup := s.newServer(c)
	defer cleanup()
	client := dialSSHServer(c, listener, "alice", gossh.PublicKeys(s.userSigner))
	_, err = client.Dial("tcp", fmt.Sprintf("%s:0", testVirtualHostname))
	c.Check(err, tc.ErrorMatches, ".*unauthorized.*")
}

func (s *sshServerSuite) TestServerAcceptsReverseTunnel(c *tc.C) {
	s.SetUpMocks(c)

	tunnelPushed := make(chan struct{})
	const tunnelPayload = "reverse tunnel payload"
	s.authenticator.EXPECT().PasswordAuthentication(gomock.Any(), "tunnel-password").DoAndReturn(
		func(ctx ssh.Context, password string) (bool, error) {
			ctx.SetValue(tunnelIDKey{}, "tunnel-id")
			return true, nil
		})
	s.tunnelTracker.EXPECT().PushTunnel(gomock.Any(), "tunnel-id", gomock.Any()).Do(
		func(_ context.Context, _ string, tunnelConn net.Conn) {
			// Checking the payload proves the handler transfers a usable tunnel
			// connection to the tracker.
			payload := make([]byte, len(tunnelPayload))
			_, err := io.ReadFull(tunnelConn, payload)
			c.Check(err, tc.ErrorIsNil)
			c.Check(string(payload), tc.Equals, tunnelPayload)
			close(tunnelPushed)
		})

	_, listener, cleanup := s.newServer(c)
	defer cleanup()

	client := dialSSHServer(c, listener, coressh.ReverseTunnelUser, gossh.Password("tunnel-password"))
	channel, requests, err := client.OpenChannel(coressh.JujuTunnelChannel, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { _ = channel.Close() })
	go gossh.DiscardRequests(requests)

	_, err = channel.Write([]byte(tunnelPayload))
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-tunnelPushed:
	case <-c.Context().Done():
		c.Error("timed out waiting for reverse tunnel to be pushed")
	}
}

func (s *sshServerSuite) TestSSHServerMaxConnections(c *tc.C) {
	s.SetUpMocks(c)
	s.authenticator.EXPECT().PublicKeyAuthentication(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx ssh.Context, key ssh.PublicKey) (bool, error) {
			c.Check(ctx.User(), tc.Equals, "ubuntu")
			c.Check(key.Marshal(), tc.DeepEquals, s.userSigner.PublicKey().Marshal())
			return true, nil
		}).Times(2 * (maxConcurrentConnections + 1))

	listener := bufconn.Listen(1024)
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
			client := dial(c, listener, config)
			clients = append(clients, client)
		}
		checkConnCount(c, maxConcurrentConnections)
		jumpServerConn, err := listener.Dial()
		c.Assert(err, tc.ErrorIsNil)

		_, _, _, err = gossh.NewClientConn(jumpServerConn, "", config)
		c.Assert(err, tc.ErrorMatches, ".*handshake failed:.*")

		// close the connections
		for _, client := range clients {
			client.Close()
		}
		checkConnCount(c, 0)
		// check the next connection is accepted
		client := dial(c, listener, config)
		client.Close()
		checkConnCount(c, 0)
	}
}

// dial returns and SSH connection that uses an in-memory transport.
func dial(c *tc.C, listener *bufconn.Listener, config *gossh.ClientConfig) *gossh.Client {
	jumpServerConn, err := listener.Dial()
	c.Assert(err, tc.ErrorIsNil)

	sshConn, newChan, reqs, err := gossh.NewClientConn(jumpServerConn, "", config)
	c.Assert(err, tc.ErrorIsNil)
	return gossh.NewClient(sshConn, newChan, reqs)
}

func (s *sshServerSuite) TestSSHWorkerReport(c *tc.C) {
	s.SetUpMocks(c)
	s.authenticator.EXPECT().PublicKeyAuthentication(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx ssh.Context, key ssh.PublicKey) (bool, error) {
			c.Check(ctx.User(), tc.Equals, "ubuntu")
			c.Check(key.Marshal(), tc.DeepEquals, s.userSigner.PublicKey().Marshal())
			return true, nil
		})

	listener := bufconn.Listen(1024)
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
	client := dial(c, listener, config)
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
