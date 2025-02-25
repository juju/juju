// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"errors"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/worker/sshserver"
	"github.com/juju/juju/worker/sshserver/mocks"
)

type workerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&workerSuite{})

func newServerWrapperWorkerConfig(
	l loggo.Logger,
	client *mocks.MockFacadeClient,
	modifier func(*sshserver.ServerWrapperWorkerConfig),
) *sshserver.ServerWrapperWorkerConfig {
	cfg := &sshserver.ServerWrapperWorkerConfig{
		NewServerWorker: func(sshserver.ServerWorkerConfig) (worker.Worker, error) { return nil, nil },
		Logger:          l,
		FacadeClient:    client,
	}

	modifier(cfg)

	return cfg
}

func (s *workerSuite) TestValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	l := loggo.GetLogger("test")
	mockFacadeClient := mocks.NewMockFacadeClient(ctrl)

	cfg := newServerWrapperWorkerConfig(l, mockFacadeClient, func(cfg *sshserver.ServerWrapperWorkerConfig) {})
	c.Assert(cfg.Validate(), jc.ErrorIsNil)

	// Test no Logger.
	cfg = newServerWrapperWorkerConfig(
		l,
		mockFacadeClient,
		func(cfg *sshserver.ServerWrapperWorkerConfig) {
			cfg.Logger = nil
		},
	)
	c.Assert(cfg.Validate(), gc.ErrorMatches, ".*is required.*")

	// Test no FacadeClient.
	cfg = newServerWrapperWorkerConfig(
		l,
		mockFacadeClient,
		func(cfg *sshserver.ServerWrapperWorkerConfig) {
			cfg.FacadeClient = nil
		},
	)
	c.Assert(cfg.Validate(), gc.ErrorMatches, ".*is required.*")

	// Test no NewServerWorker.
	cfg = newServerWrapperWorkerConfig(
		l,
		mockFacadeClient,
		func(cfg *sshserver.ServerWrapperWorkerConfig) {
			cfg.NewServerWorker = nil
		},
	)
	c.Assert(cfg.Validate(), gc.ErrorMatches, ".*is required.*")
}

func (s *workerSuite) TestSSHServerWrapperWorkerCanBeKilled(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeClient := mocks.NewMockFacadeClient(ctrl)

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

	controllerConfigWatcher := NewFakeFacadeWatcher(1, 0)
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	// Expect SSHServerHostKey to be retrieved
	mockFacadeClient.EXPECT().SSHServerHostKey().Return("key", nil).Times(1)
	// Expect WatchControllerConfig call
	mockFacadeClient.EXPECT().WatchControllerConfig().Return(controllerConfigWatcher, nil)

	// Expect config to be called just the once.
	ctrlCfg := controller.Config{
		controller.SSHServerPort:               22,
		controller.SSHMaxConcurrentConnections: 10,
	}
	mockFacadeClient.EXPECT().ControllerConfig().Return(ctrlCfg, nil).Times(1)

	cfg := sshserver.ServerWrapperWorkerConfig{
		FacadeClient: mockFacadeClient,
		Logger:       loggo.GetLogger("test"),
		NewServerWorker: func(swc sshserver.ServerWorkerConfig) (worker.Worker, error) {
			return serverWorker, nil
		},
	}
	w, err := sshserver.NewServerWrapperWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Check all workers alive properly.
	workertest.CheckAlive(c, w)
	workertest.CheckAlive(c, serverWorker)
	workertest.CheckAlive(c, controllerConfigWatcher)

	// Kill the wrapper worker.
	workertest.CleanKill(c, w)

	// Check all workers killed.
	c.Check(workertest.CheckKilled(c, w), jc.ErrorIsNil)
	c.Check(workertest.CheckKilled(c, serverWorker), jc.ErrorIsNil)
	c.Check(workertest.CheckKilled(c, controllerConfigWatcher), jc.ErrorIsNil)
}

func (s *workerSuite) TestSSHServerWrapperWorkerRestartsServerWorker(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeClient := mocks.NewMockFacadeClient(ctrl)

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

	controllerConfigWatcher := NewFakeFacadeWatcher(1, 0)
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	// Expect SSHServerHostKey to be retrieved
	mockFacadeClient.EXPECT().SSHServerHostKey().Return("key", nil).Times(1)
	// Expect WatchControllerConfig call
	mockFacadeClient.EXPECT().WatchControllerConfig().Return(controllerConfigWatcher, nil)

	// Expect first call to have port of 22 and called once.
	mockFacadeClient.EXPECT().
		ControllerConfig().
		Return(
			controller.Config{
				controller.SSHServerPort:               22,
				controller.SSHMaxConcurrentConnections: 10,
			},
			nil,
		).
		Times(1)
	// The second call, we're updating the port and should see it changes in our NewServerWorker call.
	mockFacadeClient.EXPECT().
		ControllerConfig().
		Return(
			controller.Config{
				controller.SSHServerPort:               2222,
				controller.SSHMaxConcurrentConnections: 10,
			},
			nil,
		).
		Times(1)

	startCounter := 0
	cfg := sshserver.ServerWrapperWorkerConfig{
		FacadeClient: mockFacadeClient,
		Logger:       loggo.GetLogger("test"),
		NewServerWorker: func(swc sshserver.ServerWorkerConfig) (worker.Worker, error) {
			startCounter++
			if startCounter == 1 {
				c.Assert(swc.Port, gc.Equals, 22)
			}
			if startCounter == 2 {
				c.Assert(swc.Port, gc.Equals, 2222)
			}
			return serverWorker, nil
		},
	}
	w, err := sshserver.NewServerWrapperWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Check all workers alive properly.
	workertest.CheckAlive(c, w)
	workertest.CheckAlive(c, serverWorker)
	workertest.CheckAlive(c, controllerConfigWatcher)

	// Send some changes to restart the server.
	controllerConfigWatcher.Ping()

	// Kill wrapper worker.
	workertest.CleanKill(c, w)

	// Check all workers killed.
	c.Check(workertest.CheckKilled(c, w), jc.ErrorIsNil)
	c.Check(workertest.CheckKilled(c, serverWorker), jc.ErrorIsNil)
	c.Check(workertest.CheckKilled(c, controllerConfigWatcher), jc.ErrorIsNil)

	// Expect start counter.
	// 1 for the initial start.
	// 1 for the restart.
	c.Assert(startCounter, gc.Equals, 2)
}

func (s *workerSuite) TestSSHServerWrapperWorkerErrorsOnMissingHostKey(c *gc.C) {
	l := loggo.GetLogger("test")

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	mockFacadeClient := mocks.NewMockFacadeClient(ctrl)

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

	controllerConfigWatcher := NewFakeFacadeWatcher(1, 0)
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	// Test where the host key is an empty
	mockFacadeClient.EXPECT().SSHServerHostKey().Return("", nil).Times(1)

	cfg := sshserver.ServerWrapperWorkerConfig{
		FacadeClient: mockFacadeClient,
		Logger:       l,
		NewServerWorker: func(swc sshserver.ServerWorkerConfig) (worker.Worker, error) {
			return serverWorker, nil
		},
	}
	w1, err := sshserver.NewServerWrapperWorker(cfg)
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, w1)

	err = workertest.CheckKilled(c, w1)
	c.Assert(err, gc.ErrorMatches, "jump host key is empty")

	// Test where the host key method errors
	mockFacadeClient.EXPECT().SSHServerHostKey().Return("", errors.New("state failed")).Times(1)

	cfg = sshserver.ServerWrapperWorkerConfig{
		FacadeClient: mockFacadeClient,
		Logger:       l,
		NewServerWorker: func(swc sshserver.ServerWorkerConfig) (worker.Worker, error) {
			return serverWorker, nil
		},
	}
	w2, err := sshserver.NewServerWrapperWorker(cfg)
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, w2)

	err = workertest.CheckKilled(c, w2)
	c.Assert(err, gc.ErrorMatches, "state failed")
}

// NotAWatcher is a copy from workertest.NewFakeWatcher, we needed to modify the
// Change() return type as watcher.NotifyWatcher expects Changes() watcher.NotifyChannel
// (which is just a <-chan struct{}).
type NotAWatcher struct {
	changes chan struct{}
	worker.Worker
}

// NewFakeFacadeWatcher returns a fake watcher
func NewFakeFacadeWatcher(len, preload int) NotAWatcher {
	if len < preload {
		panic("len must be larger than preload")
	}
	ch := make(chan struct{}, len)
	for i := 0; i < preload; i++ {
		ch <- struct{}{}
	}
	return NotAWatcher{
		changes: ch,
		Worker:  workertest.NewErrorWorker(nil),
	}
}

func (w NotAWatcher) Changes() watcher.NotifyChannel {
	return w.changes
}

func (w NotAWatcher) Stop() error {
	return nil
}

func (w NotAWatcher) Err() error {
	return errors.New("An error")
}

func (w *NotAWatcher) Ping() {
	w.changes <- struct{}{}
}

func (w *NotAWatcher) Close() {
	close(w.changes)
}
