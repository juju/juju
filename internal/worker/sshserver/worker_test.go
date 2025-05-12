// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"sync/atomic"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type workerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&workerSuite{})

func newServerWrapperWorkerConfig(
	c *gc.C, ctrl *gomock.Controller, modifier func(*ServerWrapperWorkerConfig),
) *ServerWrapperWorkerConfig {
	cfg := &ServerWrapperWorkerConfig{
		NewServerWorker:         func(ServerWorkerConfig) (worker.Worker, error) { return nil, nil },
		ControllerConfigService: NewMockControllerConfigService(ctrl),
		Logger:                  loggertesting.WrapCheckLog(c),
		SessionHandler:          &MockSessionHandler{},
	}

	modifier(cfg)

	return cfg
}

func (s *workerSuite) TestValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg := newServerWrapperWorkerConfig(c, ctrl, func(cfg *ServerWrapperWorkerConfig) {})
	c.Assert(cfg.Validate(), gc.IsNil)

	// Test no Logger.
	cfg = newServerWrapperWorkerConfig(
		c,
		ctrl,
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.Logger = nil
		},
	)
	c.Assert(cfg.Validate(), gc.ErrorMatches, ".*is required.*")

	// Test no NewServerWorker.
	cfg = newServerWrapperWorkerConfig(
		c,
		ctrl,
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.NewServerWorker = nil
		},
	)
	c.Assert(cfg.Validate(), gc.ErrorMatches, ".*is required.*")

	// Test no SessionHandler.
	cfg = newServerWrapperWorkerConfig(
		c,
		ctrl,
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.SessionHandler = nil
		},
	)
	c.Assert(cfg.Validate(), gc.ErrorMatches, ".*is required.*")
}

func (s *workerSuite) TestSSHServerWrapperWorkerCanBeKilled(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

	ch := make(chan []string)
	controllerConfigWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	controllerConfigService := NewMockControllerConfigService(ctrl)
	controllerConfigService.EXPECT().WatchControllerConfig().Return(controllerConfigWatcher, nil)

	// Expect config to be called just the once.
	ctrlCfg := controller.Config{
		controller.SSHServerPort:               22,
		controller.SSHMaxConcurrentConnections: 10,
	}
	controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(ctrlCfg, nil).Times(1)

	cfg := ServerWrapperWorkerConfig{
		ControllerConfigService: controllerConfigService,
		Logger:                  loggertesting.WrapCheckLog(c),
		NewServerWorker: func(swc ServerWorkerConfig) (worker.Worker, error) {
			return serverWorker, nil
		},
		SessionHandler: &stubSessionHandler{},
	}
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
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

	ch := make(chan []string)
	controllerConfigWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	controllerConfigService := NewMockControllerConfigService(ctrl)
	controllerConfigService.EXPECT().WatchControllerConfig().Return(controllerConfigWatcher, nil)

	// Expect first call to have max concurrent connections of 10 and called once on worker startup.
	controllerConfigService.EXPECT().
		ControllerConfig(gomock.Any()).
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
	controllerConfigService.EXPECT().
		ControllerConfig(gomock.Any()).
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
	controllerConfigService.EXPECT().
		ControllerConfig(gomock.Any()).
		Return(
			controller.Config{
				controller.SSHServerPort:               22,
				controller.SSHMaxConcurrentConnections: 15,
			},
			nil,
		).
		Times(1)

	var serverStarted int32
	cfg := ServerWrapperWorkerConfig{
		ControllerConfigService: controllerConfigService,
		Logger:                  loggertesting.WrapCheckLog(c),
		NewServerWorker: func(swc ServerWorkerConfig) (worker.Worker, error) {
			atomic.StoreInt32(&serverStarted, 1)
			c.Check(swc.Port, gc.Equals, 22)
			return serverWorker, nil
		},
		SessionHandler: &stubSessionHandler{},
	}
	w, err := NewServerWrapperWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Check all workers alive properly.
	workertest.CheckAlive(c, w)
	workertest.CheckAlive(c, serverWorker)
	workertest.CheckAlive(c, controllerConfigWatcher)

	c.Check(atomic.LoadInt32(&serverStarted), gc.Equals, int32(1))

	// Send some changes to restart the server (expect no changes).
	ch <- nil

	workertest.CheckAlive(c, w)

	// Send some changes to restart the server (expect the worker to restart).
	ch <- nil

	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "changes detected, stopping SSH server worker")

	// Check all workers killed.
	c.Check(workertest.CheckKilled(c, w), gc.ErrorMatches, "changes detected, stopping SSH server worker")
	c.Check(workertest.CheckKilled(c, serverWorker), jc.ErrorIsNil)
	c.Check(workertest.CheckKilled(c, controllerConfigWatcher), jc.ErrorIsNil)
}

func (s *workerSuite) TestWrapperWorkerReport(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan []string)
	controllerConfigWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	controllerConfigService := NewMockControllerConfigService(ctrl)
	controllerConfigService.EXPECT().WatchControllerConfig().Return(controllerConfigWatcher, nil)

	// Expect first call to have port of 22 and called once on worker startup.
	controllerConfigService.EXPECT().
		ControllerConfig(gomock.Any()).
		Return(
			controller.Config{
				controller.SSHServerPort:               22,
				controller.SSHMaxConcurrentConnections: 10,
			},
			nil,
		).
		Times(1)

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

	cfg := ServerWrapperWorkerConfig{
		ControllerConfigService: controllerConfigService,
		Logger:                  loggertesting.WrapCheckLog(c),
		NewServerWorker: func(swc ServerWorkerConfig) (worker.Worker, error) {
			return &reportWorker{serverWorker}, nil
		},
		SessionHandler: &stubSessionHandler{},
	}
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
