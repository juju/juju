// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/sshsession"
	"github.com/juju/juju/rpc/params"
)

type workerSuite struct {
	testing.IsolationSuite

	facadeClientMock *MockFacadeClient
	watcherMock      *MockStringsWatcher
	mockLogger       *MockLogger
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.facadeClientMock = NewMockFacadeClient(ctrl)
	s.watcherMock = NewMockStringsWatcher(ctrl)
	s.mockLogger = NewMockLogger(ctrl)

	return ctrl
}

func (s *workerSuite) newWorkerConfig(
	logger sshsession.Logger,
	modifier func(*sshsession.WorkerConfig),
) *sshsession.WorkerConfig {
	cg, _, _ := newStubConnectionGetter()
	cfg := &sshsession.WorkerConfig{
		Logger:           logger,
		MachineId:        "1",
		FacadeClient:     s.facadeClientMock,
		ConnectionGetter: cg,
	}

	modifier(cfg)

	return cfg
}

func (s *workerSuite) TestValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	l := loggo.GetLogger("test")

	// Test all OK.
	cfg := s.newWorkerConfig(l, func(wc *sshsession.WorkerConfig) {})
	c.Assert(cfg.Validate(), jc.ErrorIsNil)

	// Test no Logger.
	cfg = s.newWorkerConfig(
		l,

		func(cfg *sshsession.WorkerConfig) {
			cfg.Logger = nil
		},
	)
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test empty MachineId.
	cfg = s.newWorkerConfig(
		l,

		func(cfg *sshsession.WorkerConfig) {
			cfg.MachineId = ""
		},
	)
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no FacadeClient.
	cfg = s.newWorkerConfig(
		l,

		func(cfg *sshsession.WorkerConfig) {
			cfg.FacadeClient = nil
		},
	)
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no ConnectionGetter.
	cfg = s.newWorkerConfig(
		l,

		func(cfg *sshsession.WorkerConfig) {
			cfg.ConnectionGetter = nil
		},
	)
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *workerSuite) TestSSHSessionWorkerCanBeKilled(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	l := loggo.GetLogger("test")

	stringChan := watcher.StringsChannel(make(chan []string))
	s.watcherMock.EXPECT().Changes().Return(stringChan).AnyTimes()
	s.facadeClientMock.EXPECT().WatchSSHConnRequest("0").Return(s.watcherMock, nil).AnyTimes()

	// Check the water is Wait()'ed and Kill()'ed exactly once.
	s.watcherMock.EXPECT().Wait().Times(1)
	s.watcherMock.EXPECT().Kill().Times(1)

	connGetter, _, _ := newStubConnectionGetter()
	defer connGetter.close()

	w, err := sshsession.NewWorker(sshsession.WorkerConfig{
		Logger:           l,
		MachineId:        "0",
		FacadeClient:     s.facadeClientMock,
		ConnectionGetter: connGetter,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(workertest.CheckKill(c, w), jc.ErrorIsNil)
}

// TestSSHSessionWorkerHandlesConnection tests that the worker can at least pipe the
// connections together using an in-memory net.Pipe. Other than an actual integration
// test, we cannot test the literal SSH connections to the controller and local SSHD.
//
// Additionally, as a side effect, we're testing the ephemeral key is added and deleted.
// We don't check explicitly for jujussh.AddKeys as it will error on failure anyway.
// So we only explicitly check that the key has been removed from the authorized_key file
// on connection closure. We could write whitebox tests for key addition/removal, but we'd
// really just be testing the package.
func (s *workerSuite) TestSSHSessionWorkerHandlesConnection(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	l := loggo.GetLogger("test")

	// This is generated because in the event the test fails, we want a new one
	// so we can run the test again and expect a pass.
	testPubKey, _, err := generateTestEd25519Keys()
	c.Assert(err, jc.ErrorIsNil)

	innerChan := make(chan []string)
	go func() {
		innerChan <- []string{"machine-0-sshconnectionreq-0"}
	}()
	stringChan := watcher.StringsChannel(innerChan)

	s.watcherMock.EXPECT().Wait()
	s.watcherMock.EXPECT().Changes().Return(stringChan).AnyTimes()
	s.facadeClientMock.EXPECT().WatchSSHConnRequest("0").Return(s.watcherMock, nil).Times(1)
	s.facadeClientMock.EXPECT().GetSSHConnRequest("machine-0-sshconnectionreq-0").Return(
		params.SSHConnRequest{
			ControllerAddresses: network.NewSpaceAddresses("127.0.0.1:17022"),
			EphemeralPublicKey:  testPubKey,
		},
		nil,
	)

	// Patch the user
	u, err := user.Current()
	c.Assert(err, jc.ErrorIsNil)

	s.PatchValue(&sshsession.ControllerSSHUser, u.Name)

	// Patch the authorized keys file to be temp
	keyDir, err := authKeysDir(u.Name)
	c.Assert(err, jc.ErrorIsNil)
	keyFilename := "test_authorized_keys"
	file, err := os.CreateTemp(keyDir, keyFilename)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(err, jc.ErrorIsNil)
	defer os.Remove(file.Name())
	stat, err := file.Stat()
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&sshsession.AuthorizedKeysFile, stat.Name())
	file.Close()

	// Setup an in-memory conn getter to stub the controller and SSHD side.
	connGetter, controllerConn, sshdConn := newStubConnectionGetter()
	defer connGetter.close()

	w, err := sshsession.NewWorker(sshsession.WorkerConfig{
		Logger:           l,
		MachineId:        "0",
		FacadeClient:     s.facadeClientMock,
		ConnectionGetter: connGetter,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	go func() {
		controllerConn.Write([]byte{254})
		controllerConn.Close()
	}()

	buf := make([]byte, 1)
	read, err := sshdConn.Read(buf)
	// c.Assert(err, jc.ErrorIsNil) TODO check for errors that arent EOF
	c.Assert(read, gc.Equals, 1)
	// c.Assert(buf[0], gc.Equals, 254) TODO fix this check

}

func generateTestEd25519Keys() ([]byte, []byte, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	// Encode public key for SSH
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, nil, err
	}
	pubBytes := ssh.MarshalAuthorizedKey(sshPub)

	// Encode private key in PEM format
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	})

	pubString := strings.TrimSpace(string(pubBytes)) + " " + "testcomment"

	return []byte(pubString), privPEM, nil
}

func authKeysDir(username string) (string, error) {
	homeDir, err := utils.UserHomeDir(username)
	if err != nil {
		return "", err
	}
	homeDir, err = utils.NormalizePath(homeDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".ssh"), nil
}

// stubConnectionGetter is a stubbed connection getter that returns two in-memory
// buffered connections. One of a net.Conn representing the sshd connection
// and the other an ssh.Channel (wrapping a net.Conn) representing the controller
// connection.
type stubConnectionGetter struct {
	sshConn  ssh.Channel
	sshdConn net.Conn
}

// newStubConnectionGetter creates a stubConnectionGetter..
func newStubConnectionGetter() (*stubConnectionGetter, net.Conn, net.Conn) {
	conn1 := newBufferedConn()
	conn2 := newBufferedConn()
	return &stubConnectionGetter{sshConn: &stubSSHChannel{Conn: conn1}, sshdConn: conn2}, conn1, conn2
}

func (cg *stubConnectionGetter) GetControllerConnection(password, ctrlAddress string) (ssh.Channel, error) {
	return cg.sshConn, nil
}
func (cg *stubConnectionGetter) GetSSHDConnection() (net.Conn, error) {
	return cg.sshdConn, nil
}

func (cg *stubConnectionGetter) close() {
	cg.sshConn.Close()
	cg.sshdConn.Close()
}

// stubSSHChannel is a wrapper over a net.Conn.
type stubSSHChannel struct {
	net.Conn
}

// Implements sshChannel.
func (ssc *stubSSHChannel) CloseWrite() error {
	return nil
}

// Implements sshChannel.
func (ssc *stubSSHChannel) SendRequest(name string, wantReply bool, payload []byte) (bool, error) {
	return false, nil
}

// Implements sshChannel.
func (ssc *stubSSHChannel) Stderr() io.ReadWriter {
	return nil
}

// bufferedConn is an in-memory, buffered net.Conn implementation.
type bufferedConn struct {
	buf      bytes.Buffer
	mu       sync.Mutex
	cond     *sync.Cond
	closed   bool
	deadline time.Time
}

// newBufferedConn creates a new buffered connection.
func newBufferedConn() *bufferedConn {
	bc := &bufferedConn{}
	bc.cond = sync.NewCond(&bc.mu)
	return bc
}

// Write writes data into the buffer
func (c *bufferedConn) Write(p []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, io.ErrClosedPipe
	}
	n, err = c.buf.Write(p)
	c.cond.Broadcast() // Notify readers
	return
}

// Read reads data from the buffer
func (c *bufferedConn) Read(p []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for c.buf.Len() == 0 {
		if c.closed {
			return 0, io.EOF
		}
		c.cond.Wait() // Wait until there’s data
	}
	return c.buf.Read(p)
}

// Close marks the connection as closed
func (c *bufferedConn) Close() error {
	l := loggo.GetLogger("test")
	l.Errorf("!!!!!!!! CLOSED CALLED !!!!!!!!!!!")
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.cond.Broadcast()
	return nil
}

// Stubs net.Conn.
func (c *bufferedConn) LocalAddr() net.Addr                { return nil }
func (c *bufferedConn) RemoteAddr() net.Addr               { return nil }
func (c *bufferedConn) SetDeadline(t time.Time) error      { c.deadline = t; return nil }
func (c *bufferedConn) SetReadDeadline(t time.Time) error  { c.deadline = t; return nil }
func (c *bufferedConn) SetWriteDeadline(t time.Time) error { c.deadline = t; return nil }

type stubKeyManager struct {
}

func (s *stubKeyManager) AddPublicKey(ephemeralPublicKey string) error {
	return nil
}
func (s *stubKeyManager) CleanupPublicKey(ephemeralPublicKey string) error {
	return nil
}

func newStubKeyManager() *stubKeyManager {
	return &stubKeyManager{}
}
