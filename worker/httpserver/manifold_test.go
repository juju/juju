// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	"crypto/tls"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	dt "gopkg.in/juju/worker.v1/dependency/testing"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/httpserver"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	config               httpserver.ManifoldConfig
	manifold             dependency.Manifold
	context              dependency.Context
	agent                *mockAgent
	state                stubStateTracker
	mux                  *apiserverhttp.Mux
	raftEnabled          *mockFlag
	prometheusRegisterer stubPrometheusRegisterer
	certWatcher          stubCertWatcher
	tlsConfig            *tls.Config

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agent = &mockAgent{}
	s.state = stubStateTracker{}
	s.mux = &apiserverhttp.Mux{}
	s.raftEnabled = &mockFlag{set: true}
	s.prometheusRegisterer = stubPrometheusRegisterer{}
	s.certWatcher = stubCertWatcher{}
	s.tlsConfig = &tls.Config{}
	s.stub.ResetCalls()

	s.context = s.newContext(nil)
	s.config = httpserver.ManifoldConfig{
		AgentName:            "agent",
		CertWatcherName:      "cert-watcher",
		StateName:            "state",
		MuxName:              "mux",
		APIServerName:        "api-server",
		RaftTransportName:    "raft-transport",
		RaftEnabledName:      "raft-enabled",
		PrometheusRegisterer: &s.prometheusRegisterer,
		NewTLSConfig:         s.newTLSConfig,
		NewWorker:            s.newWorker,
	}
	s.manifold = httpserver.Manifold(s.config)
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent":          s.agent,
		"cert-watcher":   s.certWatcher.get,
		"state":          &s.state,
		"mux":            s.mux,
		"raft-enabled":   s.raftEnabled,
		"raft-transport": nil,
		"api-server":     nil,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newTLSConfig(st *state.State, getCertificate func() *tls.Certificate) (*tls.Config, error) {
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
	"agent",
	"cert-watcher",
	"state",
	"mux",
	"raft-enabled",
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

	s.stub.CheckCallNames(c, "NewTLSConfig", "NewWorker")
	newWorkerArgs := s.stub.Calls()[1].Args
	c.Assert(newWorkerArgs, gc.HasLen, 1)
	c.Assert(newWorkerArgs[0], gc.FitsTypeOf, httpserver.Config{})
	config := newWorkerArgs[0].(httpserver.Config)

	c.Assert(config, jc.DeepEquals, httpserver.Config{
		AgentConfig:          &s.agent.conf,
		PrometheusRegisterer: &s.prometheusRegisterer,
		TLSConfig:            s.tlsConfig,
		Mux:                  s.mux,
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
		func(cfg *httpserver.ManifoldConfig) { cfg.CertWatcherName = "" },
		"empty CertWatcherName not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.StateName = "" },
		"empty StateName not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.MuxName = "" },
		"empty MuxName not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.RaftTransportName = "" },
		"empty RaftTransportName not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.APIServerName = "" },
		"empty APIServerName not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.RaftEnabledName = "" },
		"empty RaftEnabledName not valid",
	}, {
		func(cfg *httpserver.ManifoldConfig) { cfg.PrometheusRegisterer = nil },
		"nil PrometheusRegisterer not valid",
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

func (s *ManifoldSuite) TestStartWithRaftDisabled(c *gc.C) {
	s.raftEnabled.set = false
	s.context = s.newContext(map[string]interface{}{
		"raft-transport": dependency.ErrMissing,
	})
	// If raft is disabled raft-transport isn't needed to start
	// up.
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)
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

type mockFlag struct {
	set bool
}

func (f *mockFlag) Check() bool {
	return f.set
}
