// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerport_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	pscontroller "github.com/juju/juju/pubsub/controller"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/controllerport"
)

type WorkerSuite struct {
	testing.IsolationSuite
	agentConfig *mockAgentConfig
	hub         *pubsub.StructuredHub
	logger      loggo.Logger
	config      controllerport.Config
	stub        testing.Stub
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.agentConfig = &mockAgentConfig{port: 232323}
	s.hub = pubsub.NewStructuredHub(nil)
	s.logger = loggo.GetLogger("controllerport_worker_test")
	s.stub.ResetCalls()

	s.logger.SetLogLevel(loggo.TRACE)

	s.config = controllerport.Config{
		AgentConfig: s.agentConfig,
		Hub:         s.hub,
		Logger:      s.logger,

		ControllerAPIPort:       232323,
		UpdateControllerAPIPort: s.updatePort,
	}
}

func (s *WorkerSuite) updatePort(port int) error {
	s.stub.MethodCall(s, "UpdatePort", port)
	return s.stub.NextErr()
}

func (s *WorkerSuite) newWorker(c *gc.C, config controllerport.Config) worker.Worker {
	w, err := controllerport.NewWorker(config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, w) })
	return w
}

func (s *WorkerSuite) TestImmediateUpdate(c *gc.C) {
	// Change the agent config so the ports are out of sync.
	s.agentConfig.port = 23456
	w := s.newWorker(c, s.config)
	workertest.CheckAlive(c, w)
	s.stub.CheckCall(c, 0, "UpdatePort", 232323)
}

func (s *WorkerSuite) TestNoChange(c *gc.C) {
	w := s.newWorker(c, s.config)
	processed, err := s.hub.Publish(pscontroller.ConfigChanged, pscontroller.ConfigChangedMessage{
		Config: controller.Config{
			"controller-api-port": 232323,
			"some-other-field":    "new value!",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-processed:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for processed")
	}
	workertest.CheckAlive(c, w)
	s.stub.CheckCallNames(c)
}

func (s *WorkerSuite) TestChange(c *gc.C) {
	w := s.newWorker(c, s.config)
	processed, err := s.hub.Publish(pscontroller.ConfigChanged, pscontroller.ConfigChangedMessage{
		Config: controller.Config{"controller-api-port": 444444},
	})
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-processed:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for processed")
	}
	s.stub.CheckCall(c, 0, "UpdatePort", 444444)
	err = workertest.CheckKilled(c, w)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrBounce)
}

func (s *WorkerSuite) TestValidate(c *gc.C) {
	type test struct {
		f      func(*controllerport.Config)
		expect string
	}
	tests := []test{{
		func(cfg *controllerport.Config) { cfg.AgentConfig = nil },
		"nil AgentConfig not valid",
	}, {
		func(cfg *controllerport.Config) { cfg.Hub = nil },
		"nil Hub not valid",
	}, {
		func(cfg *controllerport.Config) { cfg.Logger = nil },
		"nil Logger not valid",
	}, {
		func(cfg *controllerport.Config) { cfg.UpdateControllerAPIPort = nil },
		"nil UpdateControllerAPIPort not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		config := s.config
		test.f(&config)
		w, err := controllerport.NewWorker(config)
		workertest.CheckNilOrKill(c, w)
		c.Check(err, gc.ErrorMatches, test.expect)
	}
}
