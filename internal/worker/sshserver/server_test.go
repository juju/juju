// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gossh "golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/virtualhostname"
	pkitest "github.com/juju/juju/pki/test"
	params "github.com/juju/juju/rpc/params"
	jujutesting "github.com/juju/juju/testing"
)

const maxConcurrentConnections = 10
const testVirtualHostname = "1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local"

type sshServerSuite struct {
	testing.IsolationSuite

	hostKey        []byte
	publicHostKey  gossh.PublicKey
	userSigner     gossh.Signer
	facadeClient   *MockFacadeClient
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

	// Setup hostkey
	key, err := pkitest.InsecureKeyProfile()
	c.Assert(err, jc.ErrorIsNil)
	rsaKey, ok := key.(*rsa.PrivateKey)
	c.Assert(ok, jc.IsTrue)
	s.hostKey = pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(rsaKey),
		},
	)

	privateKey, err := gossh.ParsePrivateKey(s.hostKey)
	c.Assert(err, jc.ErrorIsNil)

	s.publicHostKey = privateKey.PublicKey()
}

func (s *sshServerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.facadeClient = NewMockFacadeClient(ctrl)
	s.sessionHandler = NewMockSessionHandler(ctrl)
	return ctrl
}

func newServerWorkerConfig(
	l Logger,
	j string,
	modifier func(*ServerWorkerConfig),
) *ServerWorkerConfig {
	cfg := &ServerWorkerConfig{
		Logger:      l,
		JumpHostKey: j,
	}

	modifier(cfg)

	return cfg
}

func (s *sshServerSuite) TestValidate(c *gc.C) {
	cfg := &ServerWorkerConfig{}
	l := loggo.GetLogger("test")

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

	// Test no FacadeClient.
	cfg = newServerWorkerConfig(l, "NewSSHServerListener", func(cfg *ServerWorkerConfig) {
		cfg.FacadeClient = nil
	})
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *sshServerSuite) TestSSHServerNoAuth(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.facadeClient.EXPECT().VirtualHostKey(gomock.Any()).Return(s.hostKey, nil)

	// Start the server on an in-memory listener
	listener := bufconn.Listen(1024)

	server, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggo.GetLogger("test"),
		Listener:                 listener,
		MaxConcurrentConnections: maxConcurrentConnections,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		FacadeClient:             s.facadeClient,
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

func (s *sshServerSuite) TestSSHPublicKeyHandler(c *gc.C) {
	defer s.setupMocks(c).Finish()

	listener := bufconn.Listen(1024)

	s.facadeClient.EXPECT().ListPublicKeysForModel(gomock.Any()).
		DoAndReturn(func(sshPKIAuthArgs params.ListAuthorizedKeysArgs) ([]gossh.PublicKey, error) {
			if strings.Contains(sshPKIAuthArgs.ModelUUID, "8419cd78-4993-4c3a-928e-c646226beeee") {
				return []gossh.PublicKey{s.userSigner.PublicKey()}, nil
			}
			return nil, errors.NotFound
		}).AnyTimes()
	s.facadeClient.EXPECT().VirtualHostKey(gomock.Any()).Return(s.hostKey, nil).AnyTimes()

	server, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggo.GetLogger("test"),
		Listener:                 listener,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		FacadeClient:             s.facadeClient,
		MaxConcurrentConnections: maxConcurrentConnections,
		SessionHandler:           s.sessionHandler,
	})
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, server)

	userKey, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, gc.IsNil)

	notValidSigner, err := gossh.NewSignerFromKey(userKey)
	c.Assert(err, gc.IsNil)

	tests := []struct {
		name               string
		destinationAddress string
		key                ssh.Signer
		expectSuccess      bool
	}{
		{
			name:               "valid destination model uuid and public key",
			destinationAddress: testVirtualHostname,
			key:                s.userSigner,
			expectSuccess:      true,
		},
		{
			name:               "model uuid not valid",
			destinationAddress: "1.postgresql.8419cd78-4993-4c3a-928e-eeeeeeeeeeee.juju.local",
			key:                notValidSigner,
			expectSuccess:      false,
		},
	}

	for _, test := range tests {
		c.Log(test.name)
		client := inMemoryDial(c, listener, &gossh.ClientConfig{
			User:            "",
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			Auth: []gossh.AuthMethod{
				gossh.PublicKeys(test.key),
			},
		})
		conn, err := client.Dial("tcp", fmt.Sprintf("%s:%d", test.destinationAddress, 1))
		c.Assert(err, gc.IsNil)
		// we need to establish another client connection to perform the auth in the embedded server.
		_, _, _, err = gossh.NewClientConn(
			conn,
			"",
			&gossh.ClientConfig{
				HostKeyCallback: gossh.InsecureIgnoreHostKey(),
				Auth: []gossh.AuthMethod{
					gossh.PublicKeys(test.key),
				},
			},
		)
		if !test.expectSuccess {
			c.Assert(err, gc.ErrorMatches, `.*ssh: handshake failed.*`)
		} else {
			c.Assert(err, gc.IsNil)
		}
	}
}

func (s *sshServerSuite) TestHostKeyForTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Firstly, start the server on an in-memory listener
	listener := bufconn.Listen(8 * 1024)
	s.facadeClient.EXPECT().VirtualHostKey(gomock.Any()).Return(s.hostKey, nil)
	_, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggo.GetLogger("test"),
		Listener:                 listener,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		MaxConcurrentConnections: maxConcurrentConnections,
		FacadeClient:             s.facadeClient,
		disableAuth:              true,
		SessionHandler:           s.sessionHandler,
	})
	c.Assert(err, jc.ErrorIsNil)
	// Open a client connection
	client := inMemoryDial(c, listener, &gossh.ClientConfig{
		User:            "",
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Auth: []gossh.AuthMethod{
			gossh.Password(""), // No password needed
		},
	})
	conn, err := client.Dial("tcp", fmt.Sprintf("%s:0", testVirtualHostname))
	c.Assert(err, gc.IsNil)

	// we need to establish another client connection to perform the auth in the embedded server.
	// In this way we verify the hostkey is the one coming from the facade.
	_, _, _, err = gossh.NewClientConn(
		conn,
		"",
		&gossh.ClientConfig{
			HostKeyCallback: gossh.FixedHostKey(s.publicHostKey),
			Auth: []gossh.AuthMethod{
				gossh.PublicKeys(s.userSigner),
			},
		},
	)
	c.Assert(err, gc.IsNil)

	// we now test that the connection is closed when the controller cannot fetch the unit's host key.
	s.facadeClient.EXPECT().VirtualHostKey(gomock.Any()).Return(nil, errors.New("an error"))
	client = inMemoryDial(c, listener, &gossh.ClientConfig{
		User:            "",
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Auth: []gossh.AuthMethod{
			gossh.Password(""), // No password needed
		},
	})
	_, err = client.Dial("tcp", fmt.Sprintf("%s:0", testVirtualHostname))
	c.Assert(err.Error(), gc.Equals, "ssh: rejected: connect failed (Failed to get host key)")
}

func (s *sshServerSuite) TestSSHServerMaxConnections(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.facadeClient.EXPECT().VirtualHostKey(gomock.Any()).Return(s.hostKey, nil).AnyTimes()

	listener := bufconn.Listen(1024)
	defer listener.Close()
	w, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggo.GetLogger("test"),
		Listener:                 listener,
		MaxConcurrentConnections: maxConcurrentConnections,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		FacadeClient:             s.facadeClient,
		disableAuth:              true,
		SessionHandler:           s.sessionHandler,
	})
	c.Assert(err, jc.ErrorIsNil)
	srv := w.(*ServerWorker)

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

func (s *sshServerSuite) TestSSHWorkerReport(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Firstly, start the server on an in-memory listener
	listener := bufconn.Listen(1024)
	worker, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggo.GetLogger("test"),
		Listener:                 listener,
		MaxConcurrentConnections: maxConcurrentConnections,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		FacadeClient:             s.facadeClient,
		disableAuth:              true,
		SessionHandler:           s.sessionHandler,
	})
	c.Assert(err, jc.ErrorIsNil)

	report := worker.(*ServerWorker).Report()
	c.Assert(report, gc.DeepEquals, map[string]interface{}{
		"concurrent_connections": int32(0),
	})

	// Dial the in-memory listener
	inMemoryDial(c, listener, &gossh.ClientConfig{
		User:            "",
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	})

	report = worker.(*ServerWorker).Report()
	c.Assert(report, gc.DeepEquals, map[string]interface{}{
		"concurrent_connections": int32(1),
	})
}

// inMemoryDial returns and SSH connection that uses an in-memory transport.
func inMemoryDial(c *gc.C, listener *bufconn.Listener, config *gossh.ClientConfig) *gossh.Client {
	jumpServerConn, err := listener.Dial()
	c.Assert(err, jc.ErrorIsNil)

	sshConn, newChan, reqs, err := gossh.NewClientConn(jumpServerConn, "", config)
	c.Assert(err, jc.ErrorIsNil)
	return gossh.NewClient(sshConn, newChan, reqs)
}
