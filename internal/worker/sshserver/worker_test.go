// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"sync/atomic"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher/watchertest"
)

type workerSuite struct {
	testing.IsolationSuite

	facadeClient   *MockFacadeClient
	jwtParser      *MockJWTParser
	sessionHandler *MockSessionHandler
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) newWorkerConfig(modifier func(*ServerWrapperWorkerConfig)) ServerWrapperWorkerConfig {
	cfg := &ServerWrapperWorkerConfig{
		NewServerWorker:      func(ServerWorkerConfig) (worker.Worker, error) { return nil, nil },
		Logger:               loggo.GetLogger("test"),
		FacadeClient:         s.facadeClient,
		NewSSHServerListener: newTestingSSHServerListener,
		SessionHandler:       s.sessionHandler,
		JWTParser:            s.jwtParser,
		metricsCollector:     NewMetricsCollector(),
	}

	if modifier != nil {
		modifier(cfg)
	}

	return *cfg
}

func (s *workerSuite) SetupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.facadeClient = NewMockFacadeClient(ctrl)
	s.jwtParser = NewMockJWTParser(ctrl)
	s.sessionHandler = NewMockSessionHandler(ctrl)

	return ctrl
}

func (s *workerSuite) TestValidate(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	cfg := s.newWorkerConfig(func(cfg *ServerWrapperWorkerConfig) {})
	c.Assert(cfg.Validate(), jc.ErrorIsNil)

	// Test no Logger.
	cfg = s.newWorkerConfig(
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.Logger = nil
		},
	)
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no FacadeClient.
	cfg = s.newWorkerConfig(
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.FacadeClient = nil
		},
	)
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no NewServerWorker.
	cfg = s.newWorkerConfig(
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.NewServerWorker = nil
		},
	)
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no NewSSHServerListener.
	cfg = s.newWorkerConfig(
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.NewSSHServerListener = nil
		},
	)
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no JWTParser.
	cfg = s.newWorkerConfig(
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.JWTParser = nil
		},
	)
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no SessionHandler.
	cfg = s.newWorkerConfig(
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.SessionHandler = nil
		},
	)
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	// Test no metricsCollector.
	cfg = s.newWorkerConfig(
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.metricsCollector = nil
		},
	)
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *workerSuite) TestSSHServerWrapperWorkerCanBeKilled(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

	controllerConfigWatcher := watchertest.NewMockNotifyWatcher(make(<-chan struct{}))
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	// Expect SSHServerHostKey to be retrieved
	s.facadeClient.EXPECT().SSHServerHostKey().Return("key", nil).Times(1)
	// Expect WatchControllerConfig call
	s.facadeClient.EXPECT().WatchControllerConfig().Return(controllerConfigWatcher, nil)

	// Expect config to be called just the once.
	ctrlCfg := controller.Config{
		controller.SSHServerPort:               22,
		controller.SSHMaxConcurrentConnections: 10,
	}
	s.facadeClient.EXPECT().ControllerConfig().Return(ctrlCfg, nil).Times(1)

	cfg := s.newWorkerConfig(func(swwc *ServerWrapperWorkerConfig) {
		swwc.NewServerWorker = func(swc ServerWorkerConfig) (worker.Worker, error) {
			return serverWorker, nil
		}
	})
	w, err := NewServerWrapperWorker(cfg)
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
	defer s.SetupMocks(c).Finish()

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

	watcherChan := make(chan struct{})
	controllerConfigWatcher := watchertest.NewMockNotifyWatcher(watcherChan)
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	// Expect SSHServerHostKey to be retrieved
	s.facadeClient.EXPECT().SSHServerHostKey().Return("key", nil).Times(1)
	// Expect WatchControllerConfig call
	s.facadeClient.EXPECT().WatchControllerConfig().Return(controllerConfigWatcher, nil)

	// Expect first call to have max concurrent connections of 10 and called once on worker startup.
	s.facadeClient.EXPECT().
		ControllerConfig().
		Return(
			controller.Config{
				controller.SSHServerPort:               22,
				controller.SSHMaxConcurrentConnections: 10,
			},
			nil,
		).
		Times(1)
	// The second call will be made if the worker receives changes on the watcher
	// and should should show no change and avoid restarting the worker.
	s.facadeClient.EXPECT().
		ControllerConfig().
		Return(
			controller.Config{
				controller.SSHServerPort:               22,
				controller.SSHMaxConcurrentConnections: 10,
			},
			nil,
		).
		Times(1)
	// On the third call, we're updating the max concurrent connections and should
	// see it restart the worker.
	s.facadeClient.EXPECT().
		ControllerConfig().
		Return(
			controller.Config{
				controller.SSHServerPort:               22,
				controller.SSHMaxConcurrentConnections: 15,
			},
			nil,
		).
		Times(1)

	var serverStarted int32
	cfg := s.newWorkerConfig(func(swwc *ServerWrapperWorkerConfig) {
		swwc.NewServerWorker = func(swc ServerWorkerConfig) (worker.Worker, error) {
			atomic.StoreInt32(&serverStarted, 1)
			c.Check(swc.Port, gc.Equals, 22)
			return serverWorker, nil
		}
	})
	w, err := NewServerWrapperWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Check all workers alive properly.
	workertest.CheckAlive(c, w)
	workertest.CheckAlive(c, serverWorker)
	workertest.CheckAlive(c, controllerConfigWatcher)

	c.Check(atomic.LoadInt32(&serverStarted), gc.Equals, int32(1))

	// Send some changes to restart the server (expect no changes).
	watcherChan <- struct{}{}

	workertest.CheckAlive(c, w)

	// Send some changes to restart the server (expect the worker to restart).
	watcherChan <- struct{}{}

	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "changes detected, stopping SSH server worker")

	// Check all workers killed.
	c.Check(workertest.CheckKilled(c, w), gc.ErrorMatches, "changes detected, stopping SSH server worker")
	c.Check(workertest.CheckKilled(c, serverWorker), jc.ErrorIsNil)
	c.Check(workertest.CheckKilled(c, controllerConfigWatcher), jc.ErrorIsNil)
}

func (s *workerSuite) TestSSHServerWrapperWorkerErrorsOnMissingHostKey(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

	watcherChan := make(chan struct{})
	controllerConfigWatcher := watchertest.NewMockNotifyWatcher(watcherChan)
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	// Test where the host key is an empty
	s.facadeClient.EXPECT().SSHServerHostKey().Return("", nil).Times(1)

	cfg := s.newWorkerConfig(nil)
	w1, err := NewServerWrapperWorker(cfg)
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, w1)

	err = workertest.CheckKilled(c, w1)
	c.Assert(err, gc.ErrorMatches, "jump host key is empty")

	// Test where the host key method errors
	s.facadeClient.EXPECT().SSHServerHostKey().Return("", errors.New("state failed")).Times(1)

	cfg = s.newWorkerConfig(nil)
	w2, err := NewServerWrapperWorker(cfg)
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, w2)

	err = workertest.CheckKilled(c, w2)
	c.Assert(err, gc.ErrorMatches, "state failed")
}

func (s *workerSuite) TestWrapperWorkerReport(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

	controllerConfigWatcher := watchertest.NewMockNotifyWatcher(make(<-chan struct{}))
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	// Expect SSHServerHostKey to be retrieved
	s.facadeClient.EXPECT().SSHServerHostKey().Return("key", nil).Times(1)
	// Expect WatchControllerConfig call
	s.facadeClient.EXPECT().WatchControllerConfig().Return(controllerConfigWatcher, nil)

	// Expect config to be called just the once.
	ctrlCfg := controller.Config{
		controller.SSHServerPort:               22,
		controller.SSHMaxConcurrentConnections: 10,
	}
	s.facadeClient.EXPECT().ControllerConfig().Return(ctrlCfg, nil).Times(1)

	cfg := s.newWorkerConfig(func(swwc *ServerWrapperWorkerConfig) {
		swwc.NewServerWorker = func(swc ServerWorkerConfig) (worker.Worker, error) {
			return &reportWorker{serverWorker}, nil
		}
	})
	w, err := NewServerWrapperWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Check all workers alive properly.
	workertest.CheckAlive(c, w)
	workertest.CheckAlive(c, serverWorker)
	workertest.CheckAlive(c, controllerConfigWatcher)

	// Check the wrapper worker is a reporter.
	reporter, ok := w.(worker.Reporter)
	c.Assert(ok, jc.IsTrue)

	c.Assert(reporter.Report(), jc.DeepEquals, map[string]interface{}{
		"workers": map[string]any{
			"ssh-server": map[string]any{
				"test": "test",
			},
		},
	})
}

// reportWorker is a mock worker that implements the Reporter interface.
type reportWorker struct {
	worker.Worker
}

func (r *reportWorker) Report() map[string]interface{} {
	return map[string]interface{}{
		"test": "test",
	}
}
