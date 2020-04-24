// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	"crypto/tls"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/pki"
	pkitest "github.com/juju/juju/pki/test"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/httpserver"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	authority            pki.Authority
	config               httpserver.ManifoldConfig
	manifold             dependency.Manifold
	context              dependency.Context
	state                stubStateTracker
	hub                  *pubsub.StructuredHub
	mux                  *apiserverhttp.Mux
	clock                *testclock.Clock
	prometheusRegisterer stubPrometheusRegisterer
	tlsConfig            *tls.Config
	controllerConfig     controller.Config

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)
	s.authority = authority

	s.state = stubStateTracker{}
	s.mux = &apiserverhttp.Mux{}
	s.hub = pubsub.NewStructuredHub(nil)
	s.clock = testclock.NewClock(time.Now())
	s.prometheusRegisterer = stubPrometheusRegisterer{}
	s.tlsConfig = &tls.Config{}
	s.controllerConfig = controller.Config(map[string]interface{}{
		"api-port":            1024,
		"controller-api-port": 2048,
		"api-port-open-delay": "5s",
	})
	s.stub.ResetCalls()

	s.context = s.newContext(nil)
	s.config = httpserver.ManifoldConfig{
		AgentName:            "machine-42",
		AuthorityName:        "authority",
		HubName:              "hub",
		StateName:            "state",
		MuxName:              "mux",
		APIServerName:        "api-server",
		RaftTransportName:    "raft-transport",
		Clock:                s.clock,
		PrometheusRegisterer: &s.prometheusRegisterer,
		MuxShutdownWait:      1 * time.Minute,
		LogDir:               "log-dir",
		GetControllerConfig:  s.getControllerConfig,
		NewTLSConfig:         s.newTLSConfig,
		NewWorker:            s.newWorker,
	}
	s.manifold = httpserver.Manifold(s.config)
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"authority":      s.authority,
		"state":          &s.state,
		"hub":            s.hub,
		"mux":            s.mux,
		"raft-transport": nil,
		"api-server":     nil,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) getControllerConfig(st *state.State) (controller.Config, error) {
	s.stub.MethodCall(s, "GetControllerConfig", st)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.controllerConfig, nil
}

func (s *ManifoldSuite) newTLSConfig(
	st *state.State,
	_ httpserver.SNIGetter,
) (*tls.Config, error) {
	s.stub.MethodCall(s, "NewTLSConfig", st)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.tlsConfig, nil
}

func (s *ManifoldSuite) newWorker(config httpserver.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return worker.NewRunner(worker.RunnerParams{}), nil
}

var expectedInputs = []string{
	"authority",
	"state",
	"mux",
	"hub",
	"raft-transport",
	"api-server",
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewTLSConfig", "GetControllerConfig", "NewWorker")
	newWorkerArgs := s.stub.Calls()[2].Args
	c.Assert(newWorkerArgs, gc.HasLen, 1)
	c.Assert(newWorkerArgs[0], gc.FitsTypeOf, httpserver.Config{})
	config := newWorkerArgs[0].(httpserver.Config)

	c.Assert(config, jc.DeepEquals, httpserver.Config{
		AgentName:            "machine-42",
		Clock:                s.clock,
		PrometheusRegisterer: &s.prometheusRegisterer,
		Hub:                  s.hub,
		TLSConfig:            s.tlsConfig,
		Mux:                  s.mux,
		APIPort:              1024,
		APIPortOpenDelay:     5 * time.Second,
		ControllerAPIPort:    2048,
		MuxShutdownWait:      1 * time.Minute,
		LogDir:               "log-dir",
	})
}

func (s *ManifoldSuite) TestValidate(c *gc.C) {
	type test struct {
		f      func(*httpserver.ManifoldConfig)
		expect string
	}
	tests := []test{{
		func(cfg *httpserver.ManifoldConfig) { cfg.AgentName = "" },
		"empty AgentName not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.AuthorityName = "" },
		"empty AuthorityName not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.StateName = "" },
		"empty StateName not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.MuxName = "" },
		"empty MuxName not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.MuxShutdownWait = 0 },
		"MuxShutdownWait 0s not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.LogDir = "" },
		"empty LogDir not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.RaftTransportName = "" },
		"empty RaftTransportName not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.APIServerName = "" },
		"empty APIServerName not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.PrometheusRegisterer = nil },
		"nil PrometheusRegisterer not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.GetControllerConfig = nil },
		"nil GetControllerConfig not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.NewTLSConfig = nil },
		"nil NewTLSConfig not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.NewWorker = nil },
		"nil NewWorker not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		config := s.config
		test.f(&config)
		manifold := httpserver.Manifold(config)
		w, err := manifold.Start(s.context)
		workertest.CheckNilOrKill(c, w)
		c.Check(err, gc.ErrorMatches, test.expect)
	}
}

func (s *ManifoldSuite) TestStopWorkerClosesState(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	s.state.CheckCallNames(c, "Use")

	workertest.CleanKill(c, w)
	s.state.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}
