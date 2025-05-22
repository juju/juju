// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater_test

import (
	"context"
	"maps"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/objectstore"
	watcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/agentconfigupdater"
)

type WorkerSuite struct {
	testhelpers.IsolationSuite
	agent  *mockAgent
	config agentconfigupdater.WorkerConfig

	controllerConfig        controller.Config
	controllerConifgService *MockControllerConfigService
}

func TestWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &WorkerSuite{})
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

	s.controllerConifgService.EXPECT().WatchControllerConfig(gomock.Any()).DoAndReturn(func(context.Context) (watcher.Watcher[[]string], error) {
		close(start)
		return watchertest.NewMockStringsWatcher(ch), nil
	})

	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.NotNil)

	defer workertest.CleanKill(c, w)

	select {
	case <-start:
	case <-time.After(testing.LongWait):
		c.Fatalf("waiting for watcher to start")
	}
}

func (s *WorkerSuite) TestUpdateJujuDBSnapChannel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newConfig := maps.Clone(s.controllerConfig)
	newConfig[controller.JujuDBSnapChannel] = "latest/candidate"

	w, ch, dispatched1, dispatched2 := s.runScenario(c, newConfig)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched1:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	// Snap channel is the same, worker still alive.
	workertest.CheckAlive(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched2:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateQueryTracingEnabled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newConfig := maps.Clone(s.controllerConfig)
	newConfig[controller.QueryTracingEnabled] = true

	w, ch, dispatched1, dispatched2 := s.runScenario(c, newConfig)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched1:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	// Snap channel is the same, worker still alive.
	workertest.CheckAlive(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched2:
	case <-time.After(testing.LongWait):
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
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched1:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	// Snap channel is the same, worker still alive.
	workertest.CheckAlive(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched2:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetryEnabled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newConfig := maps.Clone(s.controllerConfig)
	newConfig[controller.OpenTelemetryEnabled] = true

	w, ch, dispatched1, dispatched2 := s.runScenario(c, newConfig)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched1:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	// Snap channel is the same, worker still alive.
	workertest.CheckAlive(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched2:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetryEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newConfig := maps.Clone(s.controllerConfig)
	newConfig[controller.OpenTelemetryEndpoint] = "http://foo.bar"

	w, ch, dispatched1, dispatched2 := s.runScenario(c, newConfig)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched1:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	// Snap channel is the same, worker still alive.
	workertest.CheckAlive(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched2:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetryInsecure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newConfig := maps.Clone(s.controllerConfig)
	newConfig[controller.OpenTelemetryInsecure] = true

	w, ch, dispatched1, dispatched2 := s.runScenario(c, newConfig)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched1:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	// Snap channel is the same, worker still alive.
	workertest.CheckAlive(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched2:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetryStackTraces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newConfig := maps.Clone(s.controllerConfig)
	newConfig[controller.OpenTelemetryStackTraces] = true

	w, ch, dispatched1, dispatched2 := s.runScenario(c, newConfig)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched1:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	// Snap channel is the same, worker still alive.
	workertest.CheckAlive(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched2:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetrySampleRatio(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newConfig := maps.Clone(s.controllerConfig)
	newConfig[controller.OpenTelemetrySampleRatio] = 0.42

	w, ch, dispatched1, dispatched2 := s.runScenario(c, newConfig)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched1:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	// Snap channel is the same, worker still alive.
	workertest.CheckAlive(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched2:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetryTailSamplingThreshold(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newConfig := maps.Clone(s.controllerConfig)
	d := time.Second
	newConfig[controller.OpenTelemetryTailSamplingThreshold] = d.String()

	w, ch, dispatched1, dispatched2 := s.runScenario(c, newConfig)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched1:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	// Snap channel is the same, worker still alive.
	workertest.CheckAlive(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched2:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateObjectStoreType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newConfig := maps.Clone(s.controllerConfig)
	newConfig[controller.ObjectStoreType] = objectstore.S3Backend.String()

	w, ch, dispatched1, dispatched2 := s.runScenario(c, newConfig)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched1:
	case <-time.After(testing.ShortWait):
		c.Fatalf("event not handled")
	}

	// Snap channel is the same, worker still alive.
	workertest.CheckAlive(c, w)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("event not sent")
	}

	select {
	case <-dispatched2:
	case <-time.After(testing.ShortWait):
		c.Fatalf("event not handled")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConifgService = NewMockControllerConfigService(ctrl)

	s.agent = &mockAgent{
		conf: mockConfig{
			snapChannel:                        controller.DefaultJujuDBSnapChannel,
			queryTracingEnabled:                controller.DefaultQueryTracingEnabled,
			queryTracingThreshold:              controller.DefaultQueryTracingThreshold,
			openTelemetryEnabled:               controller.DefaultOpenTelemetryEnabled,
			openTelemetryEndpoint:              "",
			openTelemetryInsecure:              controller.DefaultOpenTelemetryInsecure,
			openTelemetryStackTraces:           controller.DefaultOpenTelemetryStackTraces,
			openTelemetrySampleRatio:           controller.DefaultOpenTelemetrySampleRatio,
			openTelemetryTailSamplingThreshold: controller.DefaultOpenTelemetryTailSamplingThreshold,
		},
	}
	s.config = agentconfigupdater.WorkerConfig{
		Agent:                              s.agent,
		ControllerConfigService:            s.controllerConifgService,
		JujuDBSnapChannel:                  controller.DefaultJujuDBSnapChannel,
		QueryTracingEnabled:                controller.DefaultQueryTracingEnabled,
		QueryTracingThreshold:              controller.DefaultQueryTracingThreshold,
		OpenTelemetryEnabled:               controller.DefaultOpenTelemetryEnabled,
		OpenTelemetryEndpoint:              "",
		OpenTelemetryInsecure:              controller.DefaultOpenTelemetryInsecure,
		OpenTelemetryStackTraces:           controller.DefaultOpenTelemetryStackTraces,
		OpenTelemetrySampleRatio:           controller.DefaultOpenTelemetrySampleRatio,
		OpenTelemetryTailSamplingThreshold: controller.DefaultOpenTelemetryTailSamplingThreshold,
		Logger:                             loggertesting.WrapCheckLog(c),
	}
	s.controllerConfig = controller.Config{
		controller.JujuDBSnapChannel:                  controller.DefaultJujuDBSnapChannel,
		controller.QueryTracingEnabled:                controller.DefaultQueryTracingEnabled,
		controller.QueryTracingThreshold:              controller.DefaultQueryTracingThreshold,
		controller.OpenTelemetryEnabled:               controller.DefaultOpenTelemetryEnabled,
		controller.OpenTelemetryEndpoint:              "",
		controller.OpenTelemetryInsecure:              controller.DefaultOpenTelemetryInsecure,
		controller.OpenTelemetryStackTraces:           controller.DefaultOpenTelemetryStackTraces,
		controller.OpenTelemetrySampleRatio:           controller.DefaultOpenTelemetrySampleRatio,
		controller.OpenTelemetryTailSamplingThreshold: controller.DefaultOpenTelemetryTailSamplingThreshold,
	}
	return ctrl
}

func (s *WorkerSuite) runScenario(c *tc.C, newConfig controller.Config) (worker.Worker, chan []string, chan struct{}, chan struct{}) {
	start := make(chan struct{})

	ch := make(chan []string)
	dispatched1 := make(chan struct{})
	dispatched2 := make(chan struct{})

	s.controllerConifgService.EXPECT().WatchControllerConfig(gomock.Any()).DoAndReturn(func(context.Context) (watcher.Watcher[[]string], error) {
		close(start)
		return watchertest.NewMockStringsWatcher(ch), nil
	})
	gomock.InOrder(
		s.controllerConifgService.EXPECT().ControllerConfig(gomock.Any()).DoAndReturn(func(ctx context.Context) (controller.Config, error) {
			close(dispatched1)
			return s.controllerConfig, nil
		}),
		s.controllerConifgService.EXPECT().ControllerConfig(gomock.Any()).DoAndReturn(func(ctx context.Context) (controller.Config, error) {
			close(dispatched2)
			return newConfig, nil
		}),
	)

	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.NotNil)

	select {
	case <-start:
	case <-time.After(testing.LongWait):
		c.Fatalf("waiting for watcher to start")
	}

	return w, ch, dispatched1, dispatched2
}
