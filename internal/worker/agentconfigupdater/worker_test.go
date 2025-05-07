// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalpubsub "github.com/juju/juju/internal/pubsub"
	controllermsg "github.com/juju/juju/internal/pubsub/controller"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/agentconfigupdater"
)

type WorkerSuite struct {
	testing.IsolationSuite
	logger logger.Logger
	agent  *mockAgent
	hub    *pubsub.StructuredHub
	config agentconfigupdater.WorkerConfig

	initialConfigMsg controllermsg.ConfigChangedMessage
}

var _ = tc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.logger = loggertesting.WrapCheckLog(c)
	s.hub = pubsub.NewStructuredHub(&pubsub.StructuredHubConfig{
		Logger: internalpubsub.WrapLogger(s.logger),
	})
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
		Hub:                                s.hub,
		JujuDBSnapChannel:                  controller.DefaultJujuDBSnapChannel,
		QueryTracingEnabled:                controller.DefaultQueryTracingEnabled,
		QueryTracingThreshold:              controller.DefaultQueryTracingThreshold,
		OpenTelemetryEnabled:               controller.DefaultOpenTelemetryEnabled,
		OpenTelemetryEndpoint:              "",
		OpenTelemetryInsecure:              controller.DefaultOpenTelemetryInsecure,
		OpenTelemetryStackTraces:           controller.DefaultOpenTelemetryStackTraces,
		OpenTelemetrySampleRatio:           controller.DefaultOpenTelemetrySampleRatio,
		OpenTelemetryTailSamplingThreshold: controller.DefaultOpenTelemetryTailSamplingThreshold,
		Logger:                             s.logger,
	}
	s.initialConfigMsg = controllermsg.ConfigChangedMessage{
		Config: controller.Config{
			controller.JujuDBSnapChannel:                  controller.DefaultJujuDBSnapChannel,
			controller.QueryTracingEnabled:                controller.DefaultQueryTracingEnabled,
			controller.QueryTracingThreshold:              controller.DefaultQueryTracingThreshold,
			controller.OpenTelemetryEnabled:               controller.DefaultOpenTelemetryEnabled,
			controller.OpenTelemetryEndpoint:              "",
			controller.OpenTelemetryInsecure:              controller.DefaultOpenTelemetryInsecure,
			controller.OpenTelemetryStackTraces:           controller.DefaultOpenTelemetryStackTraces,
			controller.OpenTelemetrySampleRatio:           controller.DefaultOpenTelemetrySampleRatio,
			controller.OpenTelemetryTailSamplingThreshold: controller.DefaultOpenTelemetryTailSamplingThreshold,
		},
	}
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
			name: "missing hub",
			config: func() agentconfigupdater.WorkerConfig {
				result := s.config
				result.Hub = nil
				return result
			},
			expectErr: "missing hub not valid",
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
		s.logger.Infof(context.TODO(), "%d: %s", i, test.name)
		config := test.config()
		err := config.Validate()
		if test.expectErr == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, jc.ErrorIs, errors.NotValid)
			c.Check(err, tc.ErrorMatches, test.expectErr)
		}
	}
}

func (s *WorkerSuite) TestNewWorkerValidatesConfig(c *tc.C) {
	config := s.config
	config.Agent = nil
	w, err := agentconfigupdater.NewWorker(config)
	c.Assert(w, tc.IsNil)
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

func (s *WorkerSuite) TestNormalStart(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestUpdateJujuDBSnapChannel(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, jc.ErrorIsNil)

	newConfig := s.initialConfigMsg
	handled, err := s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	// Snap channel is the same, worker still alive.
	workertest.CheckAlive(c, w)

	newConfig.Config[controller.JujuDBSnapChannel] = "latest/candidate"
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, tc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateQueryTracingEnabled(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, jc.ErrorIsNil)

	newConfig := s.initialConfigMsg
	handled, err := s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	// Query tracing enabled is the same, worker still alive.
	workertest.CheckAlive(c, w)

	newConfig.Config[controller.QueryTracingEnabled] = true
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, tc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateQueryTracingThreshold(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, jc.ErrorIsNil)

	newConfig := s.initialConfigMsg
	handled, err := s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	// Query tracing threshold is the same, worker still alive.
	workertest.CheckAlive(c, w)

	d := time.Second * 2
	newConfig.Config[controller.QueryTracingThreshold] = d.String()
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, tc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetryEnabled(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, jc.ErrorIsNil)

	newConfig := s.initialConfigMsg
	handled, err := s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	workertest.CheckAlive(c, w)

	newConfig.Config[controller.OpenTelemetryEnabled] = true
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, tc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetryEndpoint(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, jc.ErrorIsNil)

	newConfig := s.initialConfigMsg
	handled, err := s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	workertest.CheckAlive(c, w)

	newConfig.Config[controller.OpenTelemetryEndpoint] = "http://foo.bar"
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, tc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetryInsecure(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, jc.ErrorIsNil)

	newConfig := s.initialConfigMsg
	handled, err := s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	workertest.CheckAlive(c, w)

	newConfig.Config[controller.OpenTelemetryInsecure] = true
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, tc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetryStackTraces(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, jc.ErrorIsNil)

	newConfig := s.initialConfigMsg
	handled, err := s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	workertest.CheckAlive(c, w)

	newConfig.Config[controller.OpenTelemetryStackTraces] = true
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, tc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetrySampleRatio(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, jc.ErrorIsNil)

	newConfig := s.initialConfigMsg
	handled, err := s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	workertest.CheckAlive(c, w)

	newConfig.Config[controller.OpenTelemetrySampleRatio] = 0.42
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, tc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetryTailSamplingThreshold(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, jc.ErrorIsNil)

	newConfig := s.initialConfigMsg
	handled, err := s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	workertest.CheckAlive(c, w)

	d := time.Second
	newConfig.Config[controller.OpenTelemetryTailSamplingThreshold] = d.String()
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, tc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateObjectStoreType(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, jc.ErrorIsNil)

	newConfig := s.initialConfigMsg
	handled, err := s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	workertest.CheckAlive(c, w)

	newConfig.Config[controller.ObjectStoreType] = objectstore.S3Backend
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, tc.Equals, jworker.ErrRestartAgent)
}
