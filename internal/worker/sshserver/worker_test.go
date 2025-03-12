// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/sshserver"
)

type workerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&workerSuite{})

func newServerWrapperWorkerConfig(
	c *gc.C, ctrl *gomock.Controller, modifier func(*sshserver.ServerWrapperWorkerConfig),
) *sshserver.ServerWrapperWorkerConfig {
	cfg := &sshserver.ServerWrapperWorkerConfig{
		NewServerWorker:         func(sshserver.ServerWorkerConfig) (worker.Worker, error) { return nil, nil },
		ControllerConfigService: sshserver.NewMockControllerConfigService(ctrl),
		Logger:                  loggertesting.WrapCheckLog(c),
		NewSSHServerListener:    newTestingSSHServerListener,
	}

	modifier(cfg)

	return cfg
}

func (s *workerSuite) TestValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg := newServerWrapperWorkerConfig(c, ctrl, func(cfg *sshserver.ServerWrapperWorkerConfig) {})
	c.Assert(cfg.Validate(), gc.IsNil)

	// Test no Logger.
	cfg = newServerWrapperWorkerConfig(
		c,
		ctrl,
		func(cfg *sshserver.ServerWrapperWorkerConfig) {
			cfg.Logger = nil
		},
	)
	c.Assert(cfg.Validate(), gc.ErrorMatches, ".*is required.*")

	// Test no NewServerWorker.
	cfg = newServerWrapperWorkerConfig(
		c,
		ctrl,
		func(cfg *sshserver.ServerWrapperWorkerConfig) {
			cfg.NewServerWorker = nil
		},
	)
	c.Assert(cfg.Validate(), gc.ErrorMatches, ".*is required.*")

	// Test no NewSSHServerListener.
	cfg = newServerWrapperWorkerConfig(
		c,
		ctrl,
		func(cfg *sshserver.ServerWrapperWorkerConfig) {
			cfg.NewSSHServerListener = nil
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

	controllerConfigService := sshserver.NewMockControllerConfigService(ctrl)
	controllerConfigService.EXPECT().WatchControllerConfig().Return(controllerConfigWatcher, nil)

	cfg := sshserver.ServerWrapperWorkerConfig{
		ControllerConfigService: controllerConfigService,
		Logger:                  loggertesting.WrapCheckLog(c),
		NewServerWorker: func(swc sshserver.ServerWorkerConfig) (worker.Worker, error) {
			return serverWorker, nil
		},
		NewSSHServerListener: newTestingSSHServerListener,
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

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

	ch := make(chan []string)
	controllerConfigWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	controllerConfigService := sshserver.NewMockControllerConfigService(ctrl)
	controllerConfigService.EXPECT().WatchControllerConfig().Return(controllerConfigWatcher, nil)

	startCounter := 0
	cfg := sshserver.ServerWrapperWorkerConfig{
		ControllerConfigService: controllerConfigService,
		Logger:                  loggertesting.WrapCheckLog(c),
		NewServerWorker: func(swc sshserver.ServerWorkerConfig) (worker.Worker, error) {
			startCounter++
			return serverWorker, nil
		},
		NewSSHServerListener: newTestingSSHServerListener,
	}
	w, err := sshserver.NewServerWrapperWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Check all workers alive properly.
	workertest.CheckAlive(c, w)
	workertest.CheckAlive(c, serverWorker)
	workertest.CheckAlive(c, controllerConfigWatcher)

	// Send some changes to restart the server.
	ch <- []string{"some-config-key"}

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
