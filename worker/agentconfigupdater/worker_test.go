// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	controllermsg "github.com/juju/juju/pubsub/controller"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agentconfigupdater"
)

type WorkerSuite struct {
	testing.IsolationSuite
	logger loggo.Logger
	agent  *mockAgent
	hub    *pubsub.StructuredHub
	config agentconfigupdater.WorkerConfig

	initialConfigMsg controllermsg.ConfigChangedMessage
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.logger = loggo.GetLogger("test")
	s.hub = pubsub.NewStructuredHub(&pubsub.StructuredHubConfig{
		Logger: s.logger,
	})
	s.agent = &mockAgent{
		conf: mockConfig{
			profile:                  controller.DefaultMongoMemoryProfile,
			snapChannel:              controller.DefaultJujuDBSnapChannel,
			queryTracingEnabled:      controller.DefaultQueryTracingEnabled,
			queryTracingThreshold:    controller.DefaultQueryTracingThreshold,
			openTelemetryEnabled:     controller.DefaultOpenTelemetryEnabled,
			openTelemetryEndpoint:    "",
			openTelemetryInsecure:    controller.DefaultOpenTelemetryInsecure,
			openTelemetryStackTraces: controller.DefaultOpenTelemetryStackTraces,
		},
	}
	s.config = agentconfigupdater.WorkerConfig{
		Agent:                    s.agent,
		Hub:                      s.hub,
		MongoProfile:             controller.DefaultMongoMemoryProfile,
		JujuDBSnapChannel:        controller.DefaultJujuDBSnapChannel,
		QueryTracingEnabled:      controller.DefaultQueryTracingEnabled,
		QueryTracingThreshold:    controller.DefaultQueryTracingThreshold,
		OpenTelemetryEnabled:     controller.DefaultOpenTelemetryEnabled,
		OpenTelemetryEndpoint:    "",
		OpenTelemetryInsecure:    controller.DefaultOpenTelemetryInsecure,
		OpenTelemetryStackTraces: controller.DefaultOpenTelemetryStackTraces,
		Logger:                   s.logger,
	}
	s.initialConfigMsg = controllermsg.ConfigChangedMessage{
		Config: controller.Config{
			controller.MongoMemoryProfile:       controller.DefaultMongoMemoryProfile,
			controller.JujuDBSnapChannel:        controller.DefaultJujuDBSnapChannel,
			controller.QueryTracingEnabled:      controller.DefaultQueryTracingEnabled,
			controller.QueryTracingThreshold:    controller.DefaultQueryTracingThreshold,
			controller.OpenTelemetryEnabled:     controller.DefaultOpenTelemetryEnabled,
			controller.OpenTelemetryEndpoint:    "",
			controller.OpenTelemetryInsecure:    controller.DefaultOpenTelemetryInsecure,
			controller.OpenTelemetryStackTraces: controller.DefaultOpenTelemetryStackTraces,
		},
	}
}

func (s *WorkerSuite) TestWorkerConfig(c *gc.C) {
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
		s.logger.Infof("%d: %s", i, test.name)
		config := test.config()
		err := config.Validate()
		if test.expectErr == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, jc.Satisfies, errors.IsNotValid)
			c.Check(err, gc.ErrorMatches, test.expectErr)
		}
	}
}

func (s *WorkerSuite) TestNewWorkerValidatesConfig(c *gc.C) {
	config := s.config
	config.Agent = nil
	w, err := agentconfigupdater.NewWorker(config)
	c.Assert(w, gc.IsNil)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *WorkerSuite) TestNormalStart(c *gc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, gc.NotNil)
	c.Check(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestUpdateMongoProfile(c *gc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, gc.NotNil)
	c.Check(err, jc.ErrorIsNil)

	newConfig := s.initialConfigMsg
	handled, err := s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	// Profile the same, worker still alive.
	workertest.CheckAlive(c, w)

	newConfig.Config[controller.MongoMemoryProfile] = "new-value"
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, gc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateJujuDBSnapChannel(c *gc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, gc.NotNil)
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

	c.Assert(err, gc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateQueryTracingEnabled(c *gc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, gc.NotNil)
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

	c.Assert(err, gc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateQueryTracingThreshold(c *gc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, gc.NotNil)
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

	c.Assert(err, gc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetryEnabled(c *gc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, gc.NotNil)
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

	newConfig.Config[controller.OpenTelemetryEnabled] = true
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, gc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetryEndpoint(c *gc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, gc.NotNil)
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

	newConfig.Config[controller.OpenTelemetryEndpoint] = "http://foo.bar"
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, gc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetryInsecure(c *gc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, gc.NotNil)
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

	newConfig.Config[controller.OpenTelemetryInsecure] = true
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, gc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateOpenTelemetryStackTraces(c *gc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, gc.NotNil)
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

	newConfig.Config[controller.OpenTelemetryStackTraces] = true
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, gc.Equals, jworker.ErrRestartAgent)
}
