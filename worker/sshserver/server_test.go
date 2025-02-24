// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"

	"github.com/gliderlabs/ssh"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gossh "golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/sshserver"
	"github.com/juju/juju/worker/sshserver/mocks"
)

type sshServerSuite struct {
	testing.IsolationSuite

	userSigner gossh.Signer
}

var _ = gc.Suite(&sshServerSuite{})

func (s *sshServerSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)

	// Setup user signer
	userKey, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, gc.IsNil)

	userSigner, err := gossh.NewSignerFromKey(userKey)
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

	mockLogger := mocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()
	mockAuthenticator := mocks.NewMockAuthenticator(ctrl)

	// Firstly, start the server on an in-memory listener
	listener := bufconn.Listen(8 * 1024)

	server, err := sshserver.NewServerWorker(sshserver.ServerWorkerConfig{
		Logger:        mockLogger,
		Authenticator: mockAuthenticator,
		Listener:      listener,
	})
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, server)
	workertest.CheckAlive(c, server)

	// Dial the in-memory listener
	conn, err := listener.Dial()
	c.Assert(err, gc.IsNil)

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
	c.Assert(err, gc.IsNil)

	// Open jump connection
	client := gossh.NewClient(jumpConn, chans, terminatingReqs)
	tunnel, err := client.Dial("tcp", "1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local:20")
	c.Assert(err, gc.IsNil)

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
	c.Assert(err, gc.IsNil)

	terminatingClient := gossh.NewClient(terminatingClientConn, terminatingClientChan, terminatingReqs)
	terminatingSession, err := terminatingClient.NewSession()
	c.Assert(err, gc.IsNil)

	output, err := terminatingSession.CombinedOutput("")
	c.Assert(err, gc.IsNil)
	c.Assert(string(output), gc.Equals, "Your final destination is: 1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local as user: ubuntu\n")

	// Server isn't gracefully closed, it's forcefully closed. All connections ended
	// from server side.
	workertest.CleanKill(c, server)
}

func (s *sshServerSuite) TestSSHPublicKeyHandler(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockAuthenticator := mocks.NewMockAuthenticator(ctrl)

	listener := bufconn.Listen(8 * 1024)
	server, err := sshserver.NewServerWorker(sshserver.ServerWorkerConfig{
		Logger:        mockLogger,
		Authenticator: mockAuthenticator,
		Listener:      listener,
	})
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, server)

	userKey, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, gc.IsNil)

	notValidSigner, err := gossh.NewSignerFromKey(userKey)
	c.Assert(err, gc.IsNil)

	mockAuthenticator.EXPECT().PublicKeyAuthentication(gomock.Any(), gomock.Any()).DoAndReturn(func(userTag names.UserTag, key ssh.PublicKey) bool {
		if userTag.Name() == "alice" {
			return true
		}
		return false
	}).AnyTimes()
	mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()
	tests := []struct {
		name          string
		user          names.UserTag
		key           gossh.Signer
		expectSuccess bool
	}{
		{
			name:          "valid user and public key",
			user:          names.NewUserTag("alice"),
			key:           s.userSigner,
			expectSuccess: true,
		},
		{
			name:          "user not found",
			user:          names.NewUserTag("notfound"),
			key:           notValidSigner,
			expectSuccess: false,
		},
	}

	for _, test := range tests {
		c.Log(test.name)
		conn, err := listener.Dial()
		c.Assert(err, gc.IsNil)
		_, _, _, err = gossh.NewClientConn(
			conn,
			"",
			&gossh.ClientConfig{
				User:            test.user.Name(),
				HostKeyCallback: gossh.InsecureIgnoreHostKey(),
				Auth: []gossh.AuthMethod{
					gossh.PublicKeys(test.key),
				},
			},
		)
		if !test.expectSuccess {
			c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*ssh: handshake failed: ssh: unable to authenticate.*"))
		} else {
			c.Assert(err, gc.IsNil)
		}
	}
}
