// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	net "net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"

	network "github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/sshtunneler"
	pkitest "github.com/juju/juju/pki/test"
	state "github.com/juju/juju/state"
	jujutesting "github.com/juju/juju/testing"
)

const maxConcurrentConnections = 10

type sshServerSuite struct {
	hostKey       []byte
	publicHostKey ssh.PublicKey
	userSigner    ssh.Signer
	facadeClient  *MockFacadeClient
}

var _ = gc.Suite(&sshServerSuite{})

func (s *sshServerSuite) SetUpSuite(c *gc.C) {
	// Setup user signer
	userKey, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, jc.ErrorIsNil)

	userSigner, err := ssh.NewSignerFromKey(userKey)
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

	privateKey, err := ssh.ParsePrivateKey(s.hostKey)
	c.Assert(err, jc.ErrorIsNil)

	s.publicHostKey = privateKey.PublicKey()
}

func (s *sshServerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.facadeClient = NewMockFacadeClient(ctrl)
	return ctrl
}

func newServerWorkerConfig(
	l Logger,
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

	// Test no NewSSHServerListener.
	cfg = newServerWorkerConfig(l, "NewSSHServerListener", func(cfg *ServerWorkerConfig) {
		cfg.NewSSHServerListener = nil
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

	s.facadeClient.EXPECT().HostKeyForTarget(gomock.Any()).Return(s.hostKey, nil)

	// Start the server on an in-memory listener
	listener := bufconn.Listen(1024)

	server, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggo.GetLogger("test"),
		Listener:                 listener,
		MaxConcurrentConnections: maxConcurrentConnections,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		NewSSHServerListener:     newTestingSSHServerListener,
		FacadeClient:             s.facadeClient,
		disableAuth:              true,
		TunnelTracker:            &sshtunneler.Tracker{},
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

func (s *sshServerSuite) TestHostKeyForTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Firstly, start the server on an in-memory listener
	listener := bufconn.Listen(8 * 1024)
	s.facadeClient.EXPECT().HostKeyForTarget(gomock.Any()).Return(s.hostKey, nil)
	_, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggo.GetLogger("test"),
		Listener:                 listener,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		MaxConcurrentConnections: maxConcurrentConnections,
		NewSSHServerListener:     newTestingSSHServerListener,
		FacadeClient:             s.facadeClient,
		disableAuth:              true,
		TunnelTracker:            &sshtunneler.Tracker{},
	})
	c.Assert(err, jc.ErrorIsNil)
	// Open a client connection
	client := inMemoryDial(c, listener, &ssh.ClientConfig{
		User:            "",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	conn, err := client.Dial("tcp", "localhost:0")
	c.Assert(err, gc.IsNil)

	// we need to establish another client connection to perform the auth in the embedded server.
	// In this way we verify the hostkey is the one coming from the facade.
	_, _, _, err = ssh.NewClientConn(
		conn,
		"",
		&ssh.ClientConfig{
			HostKeyCallback: ssh.FixedHostKey(s.publicHostKey),
			Auth: []ssh.AuthMethod{
				ssh.PublicKeys(s.userSigner),
			},
		},
	)
	c.Assert(err, gc.IsNil)

	// we now test that the connection is closed when the controller cannot fetch the unit's host key.
	s.facadeClient.EXPECT().HostKeyForTarget(gomock.Any()).Return(nil, errors.New("an error"))
	client = inMemoryDial(c, listener, &ssh.ClientConfig{
		User:            "",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Auth: []ssh.AuthMethod{
			ssh.Password(""), // No password needed
		},
	})
	_, err = client.Dial("tcp", "localhost:0")
	c.Assert(err.Error(), gc.Equals, "ssh: rejected: connect failed (Failed to get host key for target localhost)")
}

func (s *sshServerSuite) TestSSHServerMaxConnections(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.facadeClient.EXPECT().HostKeyForTarget(gomock.Any()).Return(s.hostKey, nil).AnyTimes()
	// Firstly, start the server on an in-memory listener
	listener := bufconn.Listen(1024)
	_, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggo.GetLogger("test"),
		Listener:                 listener,
		MaxConcurrentConnections: maxConcurrentConnections,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		NewSSHServerListener:     newTestingSSHServerListener,
		FacadeClient:             s.facadeClient,
		disableAuth:              true,
		TunnelTracker:            &sshtunneler.Tracker{},
	})
	c.Assert(err, jc.ErrorIsNil)
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

func (s *sshServerSuite) TestSSHWorkerReport(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Firstly, start the server on an in-memory listener
	listener := bufconn.Listen(1024)
	worker, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggo.GetLogger("test"),
		Listener:                 listener,
		MaxConcurrentConnections: maxConcurrentConnections,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		NewSSHServerListener:     newTestingSSHServerListener,
		FacadeClient:             s.facadeClient,
		disableAuth:              true,
		TunnelTracker:            &sshtunneler.Tracker{},
	})
	c.Assert(err, jc.ErrorIsNil)

	report := worker.(*ServerWorker).Report()
	c.Assert(report, gc.DeepEquals, map[string]interface{}{
		"concurrent_connections": int32(0),
	})

	// Dial the in-memory listener
	inMemoryDial(c, listener, &ssh.ClientConfig{
		User:            "",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})

	report = worker.(*ServerWorker).Report()
	c.Assert(report, gc.DeepEquals, map[string]interface{}{
		"concurrent_connections": int32(1),
	})
}

type reverseTunnelSuite struct {
	tunnelTracker *sshtunneler.Tracker
	facadeClient  *MockFacadeClient

	tunnelState    *MockState
	tunnelCtrlInfo *MockControllerInfo
	tunnelClock    *MockClock
	tunnelDial     *MockSSHDial
}

var _ = gc.Suite(&reverseTunnelSuite{})

func (s *reverseTunnelSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.facadeClient = NewMockFacadeClient(ctrl)
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
		TunnelSecret: sshtunneler.TunnelSecret{
			SharedSecret: []byte("test-secret"),
			JWTAlgorithm: jwa.HS256,
		},
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

	// Setup the tunnel tracker with mock dependencies.
	// Then pass it into the server.
	s.setupTunnelTracker(c)

	server, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggo.GetLogger("test"),
		Listener:                 listener,
		MaxConcurrentConnections: maxConcurrentConnections,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		NewSSHServerListener:     newTestingSSHServerListener,
		FacadeClient:             s.facadeClient,
		disableAuth:              false,
		TunnelTracker:            s.tunnelTracker,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, server)
	workertest.CheckAlive(c, server)

	// Dial the in-memory listener
	conn, err := listener.Dial()
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	// Create a tunnel request and use the username and password
	// in the request to emulate a machine connecting to the server.
	username, password, tunnelReq := s.tunnelRequest(c)

	jumpConn, chans, terminatingReqs, err := ssh.NewClientConn(conn, "",
		&ssh.ClientConfig{
			User:            username,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Auth: []ssh.AuthMethod{
				ssh.Password(password),
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	client := ssh.NewClient(jumpConn, chans, terminatingReqs)
	defer client.Close()

	// The client will now open a custom channel that indicates this is a reverse tunnel.
	// We grab the underlying TCP connection to the server.
	var machineConn net.Conn

	s.tunnelDial.EXPECT().Dial(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(c net.Conn, s1 string, s2 ssh.Signer, hkc ssh.HostKeyCallback) (*ssh.Client, error) {
			machineConn = c
			// We return a nil SSH client because we're only
			// going to test the underlying TCP connection.
			return nil, nil
		},
	).Times(1)

	serverConn := dialReverseTunnel(c, client)
	defer serverConn.Close()

	ctx := context.Background()
	ctx, cancelF := context.WithTimeout(ctx, 1*time.Second)
	defer cancelF()
	_, err = tunnelReq.Wait(ctx)
	c.Assert(err, jc.ErrorIsNil)

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

	// Setup the tunnel tracker with mock dependencies.
	// Then pass it into the server.
	s.setupTunnelTracker(c)

	server, err := NewServerWorker(ServerWorkerConfig{
		Logger:                   loggo.GetLogger("test"),
		Listener:                 listener,
		MaxConcurrentConnections: maxConcurrentConnections,
		JumpHostKey:              jujutesting.SSHServerHostKey,
		NewSSHServerListener:     newTestingSSHServerListener,
		FacadeClient:             s.facadeClient,
		disableAuth:              false,
		TunnelTracker:            s.tunnelTracker,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, server)
	workertest.CheckAlive(c, server)

	// Dial the in-memory listener
	conn, err := listener.Dial()
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	// Connect as if we are a machine with an invalid password.
	_, _, _, err = ssh.NewClientConn(conn, "",
		&ssh.ClientConfig{
			User:            "reverse-tunnel",
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Auth: []ssh.AuthMethod{
				ssh.Password("invalid-password"),
			},
		},
	)
	c.Assert(err, gc.ErrorMatches, `ssh: handshake failed: ssh: unable to authenticate.*`)

	// Server isn't gracefully closed, it's forcefully closed. All connections ended
	// from server side.
	workertest.CleanKill(c, server)
}

// tunnelRequest is a helper test function that sets up a request for an SSH tunnel.
// It returns the username and password that were used to create the request, as
// well as the tunnel request itself that can be used to wait for the tunnel.
func (s *reverseTunnelSuite) tunnelRequest(c *gc.C) (string, string, *sshtunneler.Request) {
	var (
		username string
		password string
	)
	s.tunnelState.EXPECT().InsertSSHConnRequest(gomock.Any()).DoAndReturn(
		func(sra state.SSHConnRequestArg) error {
			username = sra.Username
			password = sra.Password
			return nil
		},
	).Return(nil).Times(1)

	s.tunnelCtrlInfo.EXPECT().Addresses().Return(network.SpaceAddresses{}, nil).Times(1)

	now := time.Now()
	s.tunnelClock.EXPECT().Now().Return(now).AnyTimes()

	tunnelReq, err := s.tunnelTracker.RequestTunnel(sshtunneler.RequestArgs{
		MachineID: "1",
		ModelUUID: "foo",
	})
	c.Assert(err, jc.ErrorIsNil)

	return username, password, tunnelReq
}

// dialReverseTunnel opens a Juju specific SSH channel for
// reverse tunnels and returns the connection for that channel.
func dialReverseTunnel(c *gc.C, client *ssh.Client) net.Conn {
	ch, in, err := client.OpenChannel("juju-tunnel", nil)
	c.Assert(err, jc.ErrorIsNil)
	go ssh.DiscardRequests(in)
	return newChannelConn(ch)
}

// inMemoryDial returns and SSH connection that uses an in-memory transport.
func inMemoryDial(c *gc.C, listener *bufconn.Listener, config *ssh.ClientConfig) *ssh.Client {
	jumpServerConn, err := listener.Dial()
	c.Assert(err, jc.ErrorIsNil)

	sshConn, newChan, reqs, err := ssh.NewClientConn(jumpServerConn, "", config)
	c.Assert(err, jc.ErrorIsNil)
	return ssh.NewClient(sshConn, newChan, reqs)
}
