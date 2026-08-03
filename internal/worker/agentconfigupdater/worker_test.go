// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater_test

import (
	"context"
	"maps"
	stdtesting "testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	watcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/agentconfigupdater"
)

type WorkerSuite struct {
	testhelpers.IsolationSuite
	agent  *mockAgent
	config agentconfigupdater.WorkerConfig

	controllerConfig        controller.Config
	controllerConfigService *MockControllerConfigService
}

func TestWorkerSuite(t *stdtesting.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *stdtesting.T) {
		tc.Run(t, &WorkerSuite{})
	})
}

func (s *WorkerSuite) TestWorkerConfig(c *tc.C) {
	for i, test := range []struct {
		name      string
		config    func() agentconfigupdater.WorkerConfig
		expectErr string
	}{
		{
			name:   "valid config",
			config: func() agentconfigupdater.WorkerConfig { return s.config },
		}, {
			name: "missing agent",
			config: func() agentconfigupdater.WorkerConfig {
				result := s.config
				result.Agent = nil
				return result
			},
			expectErr: "missing agent not valid",
		}, {
			name: "missing controller config service",
			config: func() agentconfigupdater.WorkerConfig {
				result := s.config
				result.ControllerConfigService = nil
				return result
			},
			expectErr: "missing ControllerConfigService not valid",
		}, {
			name: "missing logger",
			config: func() agentconfigupdater.WorkerConfig {
				result := s.config
				result.Logger = nil
				return result
			},
			expectErr: "missing logger not valid",
		},
	} {
		c.Logf("%d: %s", i, test.name)
		config := test.config()
		err := config.Validate()
		if test.expectErr == "" {
			c.Check(err, tc.ErrorIsNil)
		} else {
			c.Check(err, tc.ErrorIs, errors.NotValid)
			c.Check(err, tc.ErrorMatches, test.expectErr)
		}
	}
}

func (s *WorkerSuite) TestNewWorkerValidatesConfig(c *tc.C) {
	config := s.config
	config.Agent = nil
	w, err := agentconfigupdater.NewWorker(config)
	c.Assert(w, tc.IsNil)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

func (s *WorkerSuite) TestNormalStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	start := make(chan struct{})

	ch := make(chan []string)

	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).DoAndReturn(func(context.Context) (watcher.Watcher[[]string], error) {
		close(start)
		return watchertest.NewMockStringsWatcher(ch), nil
	})

	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.NotNil)

	defer workertest.CleanKill(c, w)

	select {
	case <-start:
	case <-c.Context().Done():
		c.Fatalf("waiting for watcher to start")
	}
}

func (s *WorkerSuite) TestWatcherChannelClosed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	start := make(chan struct{})
	ch := make(chan []string)
	close(ch)

	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).DoAndReturn(func(context.Context) (watcher.Watcher[[]string], error) {
		close(start)
		return watchertest.NewMockStringsWatcher(ch), nil
	})

	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.NotNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-start:
	case <-c.Context().Done():
		c.Fatalf("waiting for watcher to start")
	}

	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorMatches, "watcher channel closed")
}

func (s *WorkerSuite) TestUpdateQueryTracingEnabled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newConfig := maps.Clone(s.controllerConfig)
	newConfig[controller.QueryTracingEnabled] = true

	w, ch, dispatched1, dispatched2 := s.runScenario(c, newConfig)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{}:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched1:
	case <-c.Context().Done():
		c.Fatalf("event not handled")
	}

	// Snap channel is the same, worker still alive.
	workertest.CheckAlive(c, w)

	select {
	case ch <- []string{}:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched2:
	case <-c.Context().Done():
		c.Fatalf("event not handled")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateQueryTracingThreshold(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newConfig := maps.Clone(s.controllerConfig)
	d := time.Second * 2
	newConfig[controller.QueryTracingThreshold] = d.String()

	w, ch, dispatched1, dispatched2 := s.runScenario(c, newConfig)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{}:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched1:
	case <-c.Context().Done():
		c.Fatalf("event not handled")
	}

	// Snap channel is the same, worker still alive.
	workertest.CheckAlive(c, w)

	select {
	case ch <- []string{}:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched2:
	case <-c.Context().Done():
		c.Fatalf("event not handled")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateDqliteBusyTimeout(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newConfig := maps.Clone(s.controllerConfig)
	d := time.Second * 2
	newConfig[controller.DqliteBusyTimeout] = d.String()

	w, ch, dispatched1, dispatched2 := s.runScenario(c, newConfig)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{}:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched1:
	case <-c.Context().Done():
		c.Fatalf("event not handled")
	}

	// Snap channel is the same, worker still alive.
	workertest.CheckAlive(c, w)

	select {
	case ch <- []string{}:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched2:
	case <-c.Context().Done():
		c.Fatalf("event not handled")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)

	s.agent = &mockAgent{
		conf: mockConfig{
			queryTracingEnabled:                controller.DefaultQueryTracingEnabled,
			queryTracingThreshold:              controller.DefaultQueryTracingThreshold,
			dqliteBusyTimeout:                  controller.DefaultDqliteBusyTimeout,
			openTelemetryEnabled:               agent.DefaultOpenTelemetryEnabled,
			openTelemetryHTTPEndpoint:          "",
			openTelemetryGRPCEndpoint:          "",
			openTelemetryInsecure:              agent.DefaultOpenTelemetryInsecure,
			openTelemetryStackTraces:           agent.DefaultOpenTelemetryStackTraces,
			openTelemetrySampleRatio:           agent.DefaultOpenTelemetrySampleRatio,
			openTelemetryTailSamplingThreshold: agent.DefaultOpenTelemetryTailSamplingThreshold,
		},
	}
	s.config = agentconfigupdater.WorkerConfig{
		Agent:                   s.agent,
		ControllerConfigService: s.controllerConfigService,
		QueryTracingEnabled:     controller.DefaultQueryTracingEnabled,
		QueryTracingThreshold:   controller.DefaultQueryTracingThreshold,
		DqliteBusyTimeout:       controller.DefaultDqliteBusyTimeout,
		Logger:                  loggertesting.WrapCheckLog(c),
	}
	s.controllerConfig = controller.Config{
		controller.QueryTracingEnabled:   controller.DefaultQueryTracingEnabled,
		controller.QueryTracingThreshold: controller.DefaultQueryTracingThreshold,
		controller.DqliteBusyTimeout:     controller.DefaultDqliteBusyTimeout,
	}
	return ctrl
}

func (s *WorkerSuite) runScenario(c *tc.C, newConfig controller.Config) (worker.Worker, chan []string, chan struct{}, chan struct{}) {
	start := make(chan struct{})

	ch := make(chan []string)
	dispatched1 := make(chan struct{})
	dispatched2 := make(chan struct{})

	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).DoAndReturn(func(context.Context) (watcher.Watcher[[]string], error) {
		close(start)
		return watchertest.NewMockStringsWatcher(ch), nil
	})
	gomock.InOrder(
		s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).DoAndReturn(func(ctx context.Context) (controller.Config, error) {
			close(dispatched1)
			return s.controllerConfig, nil
		}),
		s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).DoAndReturn(func(ctx context.Context) (controller.Config, error) {
			close(dispatched2)
			return newConfig, nil
		}),
	)

	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.NotNil)

	select {
	case <-start:
	case <-c.Context().Done():
		c.Fatalf("waiting for watcher to start")
	}

	return w, ch, dispatched1, dispatched2
}
