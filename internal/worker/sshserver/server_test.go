// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	net "net"
	"strings"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"go.uber.org/mock/gomock"
	gossh "golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"

	network "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/internal/sshtunneler"
	pkitest "github.com/juju/juju/pki/test"
	params "github.com/juju/juju/rpc/params"
	state "github.com/juju/juju/state"
	jujutesting "github.com/juju/juju/testing"
)

const maxConcurrentConnections = 10
const testVirtualHostname = "1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local"

type sshServerSuite struct {
	hostKey        []byte
	publicHostKey  ssh.PublicKey
	userSigner     ssh.Signer
	facadeClient   *MockFacadeClient
	jwtParser      *MockJWTParser
	sessionHandler *MockSessionHandler
}

var _ = gc.Suite(&sshServerSuite{})

func (s *sshServerSuite) SetUpSuite(c *gc.C) {
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
	s.jwtParser = NewMockJWTParser(ctrl)
	return ctrl
}

func (s *sshServerSuite) newServerWorkerConfig(
	listener *bufconn.Listener,
	modifier func(*ServerWorkerConfig),
) ServerWorkerConfig {
	cfg := &ServerWorkerConfig{
		Logger:                   loggo.GetLogger("test"),
		Listener:                 listener,
		MaxConcurrentConnections: maxConcurrentConnections,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		NewSSHServerListener:     newTestingSSHServerListener,
		FacadeClient:             s.facadeClient,
		JWTParser:                s.jwtParser,
		SessionHandler:           s.sessionHandler,
		disableAuth:              true,
		TunnelTracker:            &sshtunneler.Tracker{},
		metricsCollector:         NewMetricsCollector(),
	}

	if modifier != nil {
		modifier(cfg)
	}

	return *cfg
}

func (s *sshServerSuite) TestValidate(c *gc.C) {
	cfg := ServerWorkerConfig{}

	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no Logger.
	cfg = s.newServerWorkerConfig(nil, func(cfg *ServerWorkerConfig) {
		cfg.Logger = nil
	})
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no JumpHostKey.
	cfg = s.newServerWorkerConfig(nil, func(cfg *ServerWorkerConfig) {
		cfg.JumpHostKey = ""
	})
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no NewSSHServerListener.
	cfg = s.newServerWorkerConfig(nil, func(cfg *ServerWorkerConfig) {
		cfg.NewSSHServerListener = nil
	})
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no FacadeClient.
	cfg = s.newServerWorkerConfig(nil, func(cfg *ServerWorkerConfig) {
		cfg.FacadeClient = nil
	})
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no JWTParser.
	cfg = s.newServerWorkerConfig(nil, func(cfg *ServerWorkerConfig) {
		cfg.JWTParser = nil
	})
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no metricsCollector.
	cfg = s.newServerWorkerConfig(nil, func(cfg *ServerWorkerConfig) {
		cfg.metricsCollector = nil
	})
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *sshServerSuite) TestSSHServerNoAuth(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.facadeClient.EXPECT().VirtualHostKey(gomock.Any()).Return(s.hostKey, nil)

	listener := bufconn.Listen(1024)
	defer listener.Close()

	server, err := NewServerWorker(s.newServerWorkerConfig(listener, nil))
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

func (s *sshServerSuite) TestPublicKeyHandler(c *gc.C) {
	defer s.setupMocks(c).Finish()

	listener := bufconn.Listen(1024)
	defer listener.Close()

	s.facadeClient.EXPECT().ListPublicKeysForModel(gomock.Any()).
		DoAndReturn(func(sshPKIAuthArgs params.ListAuthorizedKeysArgs) ([]gossh.PublicKey, error) {
			if strings.Contains(sshPKIAuthArgs.ModelUUID, "8419cd78-4993-4c3a-928e-c646226beeee") {
				return []gossh.PublicKey{s.userSigner.PublicKey()}, nil
			}
			return nil, errors.NotFound
		}).AnyTimes()
	s.facadeClient.EXPECT().VirtualHostKey(gomock.Any()).Return(s.hostKey, nil).AnyTimes()

	server, err := NewServerWorker(s.newServerWorkerConfig(listener, func(swc *ServerWorkerConfig) {
		swc.disableAuth = false
	}))
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

func (s *sshServerSuite) TestPasswordHandler(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// This token holds the public key that the user must present
	// at the terminating server proving they are the same user
	// that authenticated at JIMM.
	token, err := jwt.NewBuilder().
		Claim("ssh_public_key", base64.StdEncoding.EncodeToString(s.userSigner.PublicKey().Marshal())).
		Build()
	c.Assert(err, jc.ErrorIsNil)

	userKey, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, gc.IsNil)

	notValidSigner, err := gossh.NewSignerFromKey(userKey)
	c.Assert(err, gc.IsNil)

	tests := []struct {
		name          string
		key           ssh.Signer
		expectSuccess bool
	}{
		{
			name:          "valid key matches JWT key",
			key:           s.userSigner,
			expectSuccess: true,
		},
		{
			name:          "presented key doesn't match JWT key",
			key:           notValidSigner,
			expectSuccess: false,
		},
	}

	for _, test := range tests {
		c.Log(test.name)

		listener := bufconn.Listen(1024)
		defer listener.Close()

		s.jwtParser.EXPECT().Parse(gomock.Any(), "password").Return(token, nil)
		s.facadeClient.EXPECT().VirtualHostKey(gomock.Any()).Return(s.hostKey, nil).AnyTimes()

		server, err := NewServerWorker(s.newServerWorkerConfig(listener, func(swc *ServerWorkerConfig) {
			swc.disableAuth = false
		}))
		c.Assert(err, gc.IsNil)
		defer workertest.DirtyKill(c, server)

		client := inMemoryDial(c, listener, &gossh.ClientConfig{
			User:            "jimm",
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			Auth: []gossh.AuthMethod{
				gossh.Password("password"),
			},
		})
		defer client.Close()

		conn, err := client.Dial("tcp", fmt.Sprintf("%s:%d", testVirtualHostname, 1))
		c.Assert(err, gc.IsNil)
		defer conn.Close()

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

	listener := bufconn.Listen(1024)
	defer listener.Close()

	s.facadeClient.EXPECT().VirtualHostKey(gomock.Any()).Return(s.hostKey, nil)
	_, err := NewServerWorker(s.newServerWorkerConfig(listener, nil))
	c.Assert(err, jc.ErrorIsNil)

	// Open a client connection
	client := inMemoryDial(c, listener, &gossh.ClientConfig{
		User:            "",
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
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

	_, err := NewServerWorker(s.newServerWorkerConfig(listener, nil))
	c.Assert(err, jc.ErrorIsNil)

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
		for range maxConcurrentConnections {
			client := inMemoryDial(c, listener, config)
			_, err := client.Dial("tcp", fmt.Sprintf("%s:0", testVirtualHostname))
			c.Assert(err, jc.ErrorIsNil)
			clients = append(clients, client)
		}
		jumpServerConn, err := listener.Dial()
		c.Assert(err, jc.ErrorIsNil)

		_, _, _, err = gossh.NewClientConn(jumpServerConn, "", config)
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

func (s *sshServerSuite) TestSSHWorkerReport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	listener := bufconn.Listen(1024)
	defer listener.Close()

	worker, err := NewServerWorker(s.newServerWorkerConfig(listener, nil))
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

type reverseTunnelSuite struct {
	sshServerSuite

	tunnelTracker  *sshtunneler.Tracker
	tunnelState    *MockState
	tunnelCtrlInfo *MockControllerInfo
	tunnelClock    *MockClock
	tunnelDial     *MockSSHDial
}

var _ = gc.Suite(&reverseTunnelSuite{})

func (s *reverseTunnelSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.sshServerSuite.setupMocks(c)
	s.tunnelState = NewMockState(ctrl)
	s.tunnelCtrlInfo = NewMockControllerInfo(ctrl)
	s.tunnelClock = NewMockClock(ctrl)
	s.tunnelDial = NewMockSSHDial(ctrl)

	return ctrl
}

func (s *reverseTunnelSuite) setupTunnelTracker(c *gc.C) {
	var err error
	s.tunnelTracker, err = sshtunneler.NewTracker(sshtunneler.TrackerArgs{
		State:          s.tunnelState,
		ControllerInfo: s.tunnelCtrlInfo,
		Clock:          s.tunnelClock,
		Dialer:         s.tunnelDial,
	})
	c.Assert(err, jc.ErrorIsNil)
}

// This test is a fully in-memory test integrating the tunnel tracker
// with the SSH server, verifying that the SSH server correctly handles
// SSH connections from machines.
func (s *reverseTunnelSuite) TestSSHServerReverseTunnel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Start the server on an in-memory listener
	listener := bufconn.Listen(1024)
	defer listener.Close()

	// Setup the tunnel tracker with mock dependencies.
	// Then pass it into the server.
	s.setupTunnelTracker(c)

	server, err := NewServerWorker(s.newServerWorkerConfig(listener, func(swc *ServerWorkerConfig) {
		swc.disableAuth = false
		swc.TunnelTracker = s.tunnelTracker
	}))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, server)
	workertest.CheckAlive(c, server)

	// Dial the in-memory listener
	conn, err := listener.Dial()
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	// The client will now open a custom channel that indicates this is a reverse tunnel.
	// We grab the underlying TCP connection to the server.
	var machineConn net.Conn

	s.tunnelDial.EXPECT().Dial(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(c net.Conn, s1 string, s2 gossh.Signer, hkc gossh.HostKeyCallback) (*gossh.Client, error) {
			machineConn = c
			// We return a nil SSH client because we're only
			// going to test the underlying TCP connection.
			return nil, nil
		},
	).Times(1)

	// Create a tunnel request and use the username and password
	// in the request to emulate a machine connecting to the server.
	ctx := context.Background()
	ctx, cancelF := context.WithTimeout(ctx, 1*time.Second)
	defer cancelF()
	username, password, tunnelReq := s.tunnelRequest(ctx)

	jumpConn, chans, terminatingReqs, err := gossh.NewClientConn(conn, "",
		&gossh.ClientConfig{
			User:            username,
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			Auth: []gossh.AuthMethod{
				gossh.Password(password),
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	client := gossh.NewClient(jumpConn, chans, terminatingReqs)
	defer client.Close()

	serverConn := dialReverseTunnel(c, client)
	defer serverConn.Close()

	tunnelResp := <-tunnelReq
	c.Assert(tunnelResp.err, jc.ErrorIsNil)

	// We now have both ends of the pipe and we just need to validate that they are connected.

	testConnection := func(tx net.Conn, rx net.Conn) {
		_, err := tx.Write([]byte("ping"))
		c.Check(err, jc.ErrorIsNil)
		testBuffer := make([]byte, 4)
		_, err = rx.Read(testBuffer)
		c.Check(err, jc.ErrorIsNil)
		c.Check(string(testBuffer), gc.Equals, "ping")
	}

	testConnection(serverConn, machineConn)
	testConnection(machineConn, serverConn)

	// Server isn't gracefully closed, it's forcefully closed. All connections ended
	// from server side.
	workertest.CleanKill(c, server)
}

// TestReverseTunnelNoTunnelID tests the case where a machine
// connects to the server but we don't have a record of anyone
// requesting a tunnel for this machine.
func (s *reverseTunnelSuite) TestReverseTunnelNoTunnelID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Start the server on an in-memory listener
	listener := bufconn.Listen(1024)
	defer listener.Close()

	// Setup the tunnel tracker with mock dependencies.
	// Then pass it into the server.
	s.setupTunnelTracker(c)

	server, err := NewServerWorker(s.newServerWorkerConfig(listener, func(swc *ServerWorkerConfig) {
		swc.disableAuth = false
		swc.TunnelTracker = s.tunnelTracker
	}))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, server)
	workertest.CheckAlive(c, server)

	// Dial the in-memory listener
	conn, err := listener.Dial()
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	// Connect as if we are a machine with an invalid password.
	_, _, _, err = gossh.NewClientConn(conn, "",
		&gossh.ClientConfig{
			User:            "reverse-tunnel",
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			Auth: []gossh.AuthMethod{
				gossh.Password("invalid-password"),
			},
		},
	)
	c.Assert(err, gc.ErrorMatches, `ssh: handshake failed: ssh: unable to authenticate.*`)

	// Server isn't gracefully closed, it's forcefully closed. All connections ended
	// from server side.
	workertest.CleanKill(c, server)
}

type sshConnRequest struct {
	client *gossh.Client
	err    error
}

// tunnelRequest is a helper test function that sets up a request for an SSH tunnel.
// It returns the username and password that were used to create the request, as
// well as a channel that returns a struct with the ssh client and error.
func (s *reverseTunnelSuite) tunnelRequest(ctx context.Context) (string, string, chan sshConnRequest) {
	var (
		username string
		password string
		request  = make(chan sshConnRequest)
		// ready indicates when the SSH connection request has
		// been written to state and helps synchronize the test.
		ready = make(chan struct{})
	)
	s.tunnelState.EXPECT().InsertSSHConnRequest(gomock.Any()).DoAndReturn(
		func(sra state.SSHConnRequestArg) error {
			username = sra.Username
			password = sra.Password
			close(ready)
			return nil
		},
	).Return(nil).Times(1)

	s.tunnelState.EXPECT().MachineHostKeys(gomock.Any(), gomock.Any()).Return([]string{}, nil).Times(1)

	s.tunnelCtrlInfo.EXPECT().Addresses().Return(network.SpaceAddresses{}, nil).Times(1)

	now := time.Now()
	s.tunnelClock.EXPECT().Now().Return(now).AnyTimes()

	go func() {
		client, err := s.tunnelTracker.RequestTunnel(ctx, sshtunneler.RequestArgs{
			MachineID: "1",
			ModelUUID: "foo",
		})
		request <- sshConnRequest{client: client, err: err}
	}()

	<-ready
	return username, password, request
}

// dialReverseTunnel opens a Juju specific SSH channel for
// reverse tunnels and returns the connection for that channel.
func dialReverseTunnel(c *gc.C, client *gossh.Client) net.Conn {
	ch, in, err := client.OpenChannel(jujuTunnelChannel, nil)
	c.Assert(err, jc.ErrorIsNil)
	go gossh.DiscardRequests(in)
	return newChannelConn(ch)
}

// inMemoryDial returns and SSH connection that uses an in-memory transport.
func inMemoryDial(c *gc.C, listener *bufconn.Listener, config *gossh.ClientConfig) *gossh.Client {
	jumpServerConn, err := listener.Dial()
	c.Assert(err, jc.ErrorIsNil)

	sshConn, newChan, reqs, err := gossh.NewClientConn(jumpServerConn, "", config)
	c.Assert(err, jc.ErrorIsNil)
	return gossh.NewClient(sshConn, newChan, reqs)
}
