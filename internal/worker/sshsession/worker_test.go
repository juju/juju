// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession_test

import (
	"bytes"
	"io"
	"net"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
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

	facadeClientMock        *MockFacadeClient
	watcherMock             *MockStringsWatcher
	ephemeralkeyUpdaterMock *MockEphemeralKeysUpdater
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.facadeClientMock = NewMockFacadeClient(ctrl)
	s.watcherMock = NewMockStringsWatcher(ctrl)
	s.ephemeralkeyUpdaterMock = NewMockEphemeralKeysUpdater(ctrl)

	return ctrl
}

func (s *workerSuite) newWorkerConfig(
	logger sshsession.Logger,
	modifier func(*sshsession.WorkerConfig),
) *sshsession.WorkerConfig {
	cg, _, _ := newStubConnectionGetter()

	cfg := &sshsession.WorkerConfig{
		Logger:               logger,
		MachineId:            "1",
		FacadeClient:         s.facadeClientMock,
		ConnectionGetter:     cg,
		EphemeralKeysUpdater: s.ephemeralkeyUpdaterMock,
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

	// Test no EphemeralKeysUpdater.
	cfg = s.newWorkerConfig(
		l,

		func(cfg *sshsession.WorkerConfig) {
			cfg.EphemeralKeysUpdater = nil
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
		Logger:               l,
		MachineId:            "0",
		FacadeClient:         s.facadeClientMock,
		ConnectionGetter:     connGetter,
		EphemeralKeysUpdater: s.ephemeralkeyUpdaterMock,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(workertest.CheckKill(c, w), jc.ErrorIsNil)
}

// TestSSHSessionWorkerHandlesConnection tests that the worker can at least pipe the
// connections together using an in-memory net.Pipe. Other than an actual integration
// test, we cannot test the literal SSH connections to the controller and local SSHD.
func (s *workerSuite) TestSSHSessionWorkerHandlesConnectionPipesData(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	l := loggo.GetLogger("test")

	innerChan := make(chan []string)
	go func() {
		innerChan <- []string{"machine-0-sshconnectionreq-0"}
	}()
	stringChan := watcher.StringsChannel(innerChan)

	s.watcherMock.EXPECT().Wait().AnyTimes()
	s.watcherMock.EXPECT().Kill().AnyTimes()
	s.watcherMock.EXPECT().Changes().Return(stringChan).AnyTimes()

	s.facadeClientMock.EXPECT().WatchSSHConnRequest("0").Return(s.watcherMock, nil).Times(1)
	s.facadeClientMock.EXPECT().GetSSHConnRequest("machine-0-sshconnectionreq-0").Return(
		params.SSHConnRequest{
			ControllerAddresses: network.NewSpaceAddresses("127.0.0.1:17022"),
			EphemeralPublicKey:  []byte{1},
		},
		nil,
	)

	s.ephemeralkeyUpdaterMock.EXPECT().AddEphemeralKey(string([]byte{1})).Times(1)
	s.ephemeralkeyUpdaterMock.EXPECT().RemoveEphemeralKey(string([]byte{1})).Times(1)

	// Setup an in-memory conn getter to stub the controller and SSHD side.
	connGetter, controllerConn, sshdConn := newStubConnectionGetter()
	defer connGetter.close()

	w, err := sshsession.NewWorker(sshsession.WorkerConfig{
		Logger:               l,
		MachineId:            "0",
		FacadeClient:         s.facadeClientMock,
		ConnectionGetter:     connGetter,
		EphemeralKeysUpdater: s.ephemeralkeyUpdaterMock,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	go func() {
		controllerConn.Write([]byte{254})
		controllerConn.Close()
	}()

	buf := make([]byte, 1)
	read, err := sshdConn.Read(buf)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(read, gc.Equals, 1)
	c.Assert(buf[0], gc.Equals, uint8(254))
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
