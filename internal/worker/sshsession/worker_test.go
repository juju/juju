// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"context"
	"io"
	"net"
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gossh "golang.org/x/crypto/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/pki/test"
	"github.com/juju/juju/rpc/params"
)

var (
	sshdConfigTemplate = `
# This is the sshd server system-wide configuration file.  See
# sshd_config(5) for more information.

# This sshd was compiled with PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games

# The strategy used for options in the default sshd_config shipped with
# OpenSSH is to specify options with their default value where
# possible, but leave them commented.  Uncommented options override the
# default value.

Include /etc/ssh/sshd_config.d/*.conf

Port 17023
#AddressFamily any
#ListenAddress 0.0.0.0
`
)

type workerSuite struct {
	testing.IsolationSuite

	facadeClient        *MockFacadeClient
	watcher             *MockStringsWatcher
	ephemeralkeyUpdater *MockEphemeralKeysUpdater
	connectionGetter    *MockConnectionGetter
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.facadeClient = NewMockFacadeClient(ctrl)
	s.watcher = NewMockStringsWatcher(ctrl)
	s.ephemeralkeyUpdater = NewMockEphemeralKeysUpdater(ctrl)
	s.connectionGetter = NewMockConnectionGetter(ctrl)

	return ctrl
}

func (s *workerSuite) newWorkerConfig(
	logger Logger,
	modifier func(*WorkerConfig),
) *WorkerConfig {

	cfg := &WorkerConfig{
		Logger:               logger,
		MachineId:            "1",
		FacadeClient:         s.facadeClient,
		ConnectionGetter:     s.connectionGetter,
		EphemeralKeysUpdater: s.ephemeralkeyUpdater,
	}

	modifier(cfg)

	return cfg
}

func (s *workerSuite) TestValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	l := loggo.GetLogger("test")

	// Test all OK.
	cfg := s.newWorkerConfig(l, func(wc *WorkerConfig) {})
	c.Assert(cfg.Validate(), jc.ErrorIsNil)

	// Test no Logger.
	cfg = s.newWorkerConfig(
		l,

		func(cfg *WorkerConfig) {
			cfg.Logger = nil
		},
	)
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test empty MachineId.
	cfg = s.newWorkerConfig(
		l,

		func(cfg *WorkerConfig) {
			cfg.MachineId = ""
		},
	)
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no FacadeClient.
	cfg = s.newWorkerConfig(
		l,

		func(cfg *WorkerConfig) {
			cfg.FacadeClient = nil
		},
	)
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no ConnectionGetter.
	cfg = s.newWorkerConfig(
		l,

		func(cfg *WorkerConfig) {
			cfg.ConnectionGetter = nil
		},
	)
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no EphemeralKeysUpdater.
	cfg = s.newWorkerConfig(
		l,

		func(cfg *WorkerConfig) {
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
	s.watcher.EXPECT().Changes().Return(stringChan).AnyTimes()
	s.facadeClient.EXPECT().WatchSSHConnRequest("0").Return(s.watcher, nil).AnyTimes()

	// Check the water is Wait()'ed and Kill()'ed exactly once.
	s.watcher.EXPECT().Wait().Times(1)
	s.watcher.EXPECT().Kill().Times(1)

	w, err := NewWorker(WorkerConfig{
		Logger:               l,
		MachineId:            "0",
		FacadeClient:         s.facadeClient,
		ConnectionGetter:     s.connectionGetter,
		EphemeralKeysUpdater: s.ephemeralkeyUpdater,
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

	connID := "machine-0-sshconnectionreq-0"

	testKey, err := test.InsecureKeyProfile()
	c.Assert(err, jc.ErrorIsNil)
	ephemeralPublicKey, err := gossh.NewPublicKey(testKey.Public())
	c.Assert(err, jc.ErrorIsNil)

	innerChan := make(chan []string)
	go func() {
		innerChan <- []string{connID}
	}()
	stringChan := watcher.StringsChannel(innerChan)

	s.watcher.EXPECT().Wait().AnyTimes()
	s.watcher.EXPECT().Kill().AnyTimes()
	s.watcher.EXPECT().Changes().Return(stringChan).AnyTimes()

	s.facadeClient.EXPECT().WatchSSHConnRequest("0").Return(s.watcher, nil)
	s.facadeClient.EXPECT().GetSSHConnRequest(connID).Return(
		params.SSHConnRequest{
			ControllerAddresses: network.NewSpaceAddresses("127.0.0.1:17022"),
			EphemeralPublicKey:  ephemeralPublicKey.Marshal(),
		},
		nil,
	)

	s.ephemeralkeyUpdater.EXPECT().AddEphemeralKey(ephemeralPublicKey, connID)
	s.ephemeralkeyUpdater.EXPECT().RemoveEphemeralKey(ephemeralPublicKey)

	// Setup an in-memory conn getter to stub the controller and SSHD side.
	connSSHD, workerConnSSHD := net.Pipe()
	workerControllerConn, controllerConn := net.Pipe()

	s.connectionGetter.EXPECT().GetSSHDConnection().Return(workerConnSSHD, nil)
	s.connectionGetter.EXPECT().GetControllerConnection(gomock.Any(), gomock.Any()).Return(workerControllerConn, nil)

	w, err := NewWorker(WorkerConfig{
		Logger:               l,
		MachineId:            "0",
		FacadeClient:         s.facadeClient,
		ConnectionGetter:     s.connectionGetter,
		EphemeralKeysUpdater: s.ephemeralkeyUpdater,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	go func() {
		// use a different error var to avoid shadowing the others
		// and causing a race condition.
		_, err := controllerConn.Write([]byte("hello world"))
		c.Check(err, jc.ErrorIsNil)
		err = controllerConn.Close()
		c.Check(err, jc.ErrorIsNil)
	}()

	err = connSSHD.SetReadDeadline(time.Now().Add(1 * time.Second))
	c.Assert(err, jc.ErrorIsNil)
	buf, err := io.ReadAll(connSSHD)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(buf, gc.DeepEquals, []byte("hello world"))
}

// TestContextCancelledIsPropagated tests that the context is cancelled and
// the connections are closed.
func (s *workerSuite) TestContextCancelledIsPropagated(c *gc.C) {
	defer s.setupMocks(c).Finish()
	l := loggo.GetLogger("test")
	connID := "machine-0-sshconnectionreq-0"
	innerChan := make(chan []string)
	stringChan := watcher.StringsChannel(innerChan)

	testKey, err := test.InsecureKeyProfile()
	c.Assert(err, jc.ErrorIsNil)
	ephemeralPublicKey, err := gossh.NewPublicKey(testKey.Public())
	c.Assert(err, jc.ErrorIsNil)

	s.watcher.EXPECT().Wait().AnyTimes()
	s.watcher.EXPECT().Kill().AnyTimes()
	s.watcher.EXPECT().Changes().Return(stringChan).AnyTimes()

	s.facadeClient.EXPECT().WatchSSHConnRequest("0").Return(s.watcher, nil)
	s.facadeClient.EXPECT().GetSSHConnRequest(connID).Return(
		params.SSHConnRequest{
			ControllerAddresses: network.NewSpaceAddresses("127.0.0.1:17022"),
			EphemeralPublicKey:  ephemeralPublicKey.Marshal(),
		},
		nil,
	)
	s.ephemeralkeyUpdater.EXPECT().AddEphemeralKey(ephemeralPublicKey, connID)
	s.ephemeralkeyUpdater.EXPECT().RemoveEphemeralKey(ephemeralPublicKey)
	connSSHD, workerConnSSHD := net.Pipe()
	workerControllerConn, controllerConn := net.Pipe()

	s.connectionGetter.EXPECT().GetSSHDConnection().Return(workerConnSSHD, nil)
	s.connectionGetter.EXPECT().GetControllerConnection(gomock.Any(), gomock.Any()).Return(workerControllerConn, nil)
	w, err := NewWorker(WorkerConfig{
		Logger:               l,
		MachineId:            "0",
		FacadeClient:         s.facadeClient,
		ConnectionGetter:     s.connectionGetter,
		EphemeralKeysUpdater: s.ephemeralkeyUpdater,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)
	sessionWorker, ok := w.(*sshSessionWorker)
	c.Assert(ok, gc.Equals, true)
	ctx, cancel := context.WithCancel(context.Background())
	doneChan := make(chan struct{})
	go func() {
		_ = sessionWorker.handleConnection(ctx, connID)
		close(doneChan)
	}()

	// Cancel the context to simulate a cancellation.
	cancel()
	select {
	case <-doneChan:
		// check both ends of the pipe are closed
		_, err := connSSHD.Read(make([]byte, 1))
		c.Assert(err, jc.ErrorIs, io.EOF) // error when remote end is closed
		_, err = controllerConn.Read(make([]byte, 1))
		c.Assert(err, jc.ErrorIs, io.EOF) // error when remote end is closed
	case <-time.After(testing.ShortWait):
		c.Errorf("timed out waiting for connection to be closed")
	}
}

func (s *workerSuite) TestSSHSessionWorkerMultipleConnections(c *gc.C) {
	defer s.setupMocks(c).Finish()

	l := loggo.GetLogger("test")

	connID := "machine-0-sshconnectionreq-0"

	testKey, err := test.InsecureKeyProfile()
	c.Assert(err, jc.ErrorIsNil)
	ephemeralPublicKey, err := gossh.NewPublicKey(testKey.Public())
	c.Assert(err, jc.ErrorIsNil)

	innerChan := make(chan []string)
	go func() {
		innerChan <- []string{connID, connID}
	}()
	stringChan := watcher.StringsChannel(innerChan)

	s.watcher.EXPECT().Wait().AnyTimes()
	s.watcher.EXPECT().Kill().AnyTimes()
	s.watcher.EXPECT().Changes().Return(stringChan).AnyTimes()

	s.facadeClient.EXPECT().WatchSSHConnRequest("0").Return(s.watcher, nil)
	s.facadeClient.EXPECT().GetSSHConnRequest(connID).Return(
		params.SSHConnRequest{
			ControllerAddresses: network.NewSpaceAddresses("127.0.0.1:17022"),
			EphemeralPublicKey:  ephemeralPublicKey.Marshal(),
		},
		nil,
	).Times(2)

	s.ephemeralkeyUpdater.EXPECT().AddEphemeralKey(ephemeralPublicKey, connID).Times(2)
	s.ephemeralkeyUpdater.EXPECT().RemoveEphemeralKey(ephemeralPublicKey).Times(2)

	// Setup an in-memory conn getter to stub the controller and SSHD side.
	connSSHD1, workerConnSSHD1 := net.Pipe()
	workerControllerConn1, controllerConn1 := net.Pipe()
	s.connectionGetter.EXPECT().GetSSHDConnection().Return(workerConnSSHD1, nil)
	s.connectionGetter.EXPECT().GetControllerConnection(gomock.Any(), gomock.Any()).Return(workerControllerConn1, nil)

	connSSHD2, workerConnSSHD2 := net.Pipe()
	workerControllerConn2, controllerConn2 := net.Pipe()

	s.connectionGetter.EXPECT().GetSSHDConnection().Return(workerConnSSHD2, nil)
	s.connectionGetter.EXPECT().GetControllerConnection(gomock.Any(), gomock.Any()).Return(workerControllerConn2, nil)

	w, err := NewWorker(WorkerConfig{
		Logger:               l,
		MachineId:            "0",
		FacadeClient:         s.facadeClient,
		ConnectionGetter:     s.connectionGetter,
		EphemeralKeysUpdater: s.ephemeralkeyUpdater,
	})
	c.Assert(err, jc.ErrorIsNil)

	defer workertest.CleanKill(c, w)
	// test the second pipe is working even if the first one is blocked.
	go func() {
		// use a different error var to avoid shadowing the others
		// and causing a race condition.
		_, err := controllerConn2.Write([]byte("hello world"))
		c.Check(err, jc.ErrorIsNil)
		err = controllerConn2.Close()
		c.Check(err, jc.ErrorIsNil)
	}()

	err = connSSHD2.SetReadDeadline(time.Now().Add(1 * time.Second))
	c.Assert(err, jc.ErrorIsNil)
	buf, err := io.ReadAll(connSSHD2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(buf, gc.DeepEquals, []byte("hello world"))

	// test the first pipe is working.
	go func() {
		// use a different error var to avoid shadowing the others
		// and causing a race condition.
		_, err := controllerConn1.Write([]byte("hello world"))
		c.Check(err, jc.ErrorIsNil)
		err = controllerConn1.Close()
		c.Check(err, jc.ErrorIsNil)
	}()

	err = connSSHD1.SetReadDeadline(time.Now().Add(1 * time.Second))
	c.Assert(err, jc.ErrorIsNil)
	buf, err = io.ReadAll(connSSHD1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(buf, gc.DeepEquals, []byte("hello world"))
}

// TestConnectionGetterGetLocalSSHPort tests the local SSHD port can be retrieved.
// This function never actually fails, and instead defaults to 22. So we create
// a temp file with a very distinct port number to find.
func (s *workerSuite) TestConnectionGetterGetLocalSSHPort(c *gc.C) {
	file, err := os.CreateTemp("", "test-ssd-config")
	c.Assert(err, gc.IsNil)
	defer os.Remove(file.Name())

	_, err = file.Write([]byte(sshdConfigTemplate))
	c.Assert(err, gc.IsNil)

	l := loggo.GetLogger("test")
	cg := newConnectionGetter(l)
	port := cg.getLocalSSHPort(file.Name())
	c.Assert(port, gc.Equals, "17023")
}
