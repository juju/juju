// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession_test

import (
	"io"
	"net"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/sshsession"
	"github.com/juju/juju/rpc/params"
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
	logger sshsession.Logger,
	modifier func(*sshsession.WorkerConfig),
) *sshsession.WorkerConfig {

	cfg := &sshsession.WorkerConfig{
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
	s.watcher.EXPECT().Changes().Return(stringChan).AnyTimes()
	s.facadeClient.EXPECT().WatchSSHConnRequest("0").Return(s.watcher, nil).AnyTimes()

	// Check the water is Wait()'ed and Kill()'ed exactly once.
	s.watcher.EXPECT().Wait().Times(1)
	s.watcher.EXPECT().Kill().Times(1)

	w, err := sshsession.NewWorker(sshsession.WorkerConfig{
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

	innerChan := make(chan []string)
	go func() {
		innerChan <- []string{"machine-0-sshconnectionreq-0"}
	}()
	stringChan := watcher.StringsChannel(innerChan)

	s.watcher.EXPECT().Wait().AnyTimes()
	s.watcher.EXPECT().Kill().AnyTimes()
	s.watcher.EXPECT().Changes().Return(stringChan).AnyTimes()

	s.facadeClient.EXPECT().WatchSSHConnRequest("0").Return(s.watcher, nil)
	s.facadeClient.EXPECT().GetSSHConnRequest("machine-0-sshconnectionreq-0").Return(
		params.SSHConnRequest{
			ControllerAddresses: network.NewSpaceAddresses("127.0.0.1:17022"),
			EphemeralPublicKey:  []byte{1},
		},
		nil,
	)

	s.ephemeralkeyUpdater.EXPECT().AddEphemeralKey(string([]byte{1}))
	s.ephemeralkeyUpdater.EXPECT().RemoveEphemeralKey(string([]byte{1}))

	// Setup an in-memory conn getter to stub the controller and SSHD side.
	connSSHD, workerConnSSHD := net.Pipe()
	workerControllerConn, controllerConn := net.Pipe()

	s.connectionGetter.EXPECT().GetSSHDConnection().Return(workerConnSSHD, nil)
	s.connectionGetter.EXPECT().GetControllerConnection(gomock.Any(), gomock.Any()).Return(workerControllerConn, nil)

	w, err := sshsession.NewWorker(sshsession.WorkerConfig{
		Logger:               l,
		MachineId:            "0",
		FacadeClient:         s.facadeClient,
		ConnectionGetter:     s.connectionGetter,
		EphemeralKeysUpdater: s.ephemeralkeyUpdater,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	go func() {
		controllerConn.Write([]byte("hello world"))
		controllerConn.Close()
	}()

	buf, err := io.ReadAll(connSSHD)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(buf, gc.DeepEquals, []byte("hello world"))
}
