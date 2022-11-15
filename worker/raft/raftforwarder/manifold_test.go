// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftforwarder_test

import (
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/raft/raftforwarder"
)

type manifoldSuite struct {
	testing.IsolationSuite

	context  dependency.Context
	manifold dependency.Manifold
	config   raftforwarder.ManifoldConfig

	raft   *raft.Raft
	hub    *pubsub.StructuredHub
	logger loggo.Logger
	worker worker.Worker

	stub testing.Stub
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub.ResetCalls()

	s.raft = &raft.Raft{}
	s.hub = &pubsub.StructuredHub{}

	s.worker = &mockWorker{}
	s.logger = loggo.GetLogger("juju.worker.raftforwarder_test")

	s.context = s.newContext(nil)
	s.config = raftforwarder.ManifoldConfig{
		RaftName:             "raft",
		CentralHubName:       "hub",
		RequestTopic:         "test.request",
		Logger:               &s.logger,
		PrometheusRegisterer: &noopRegisterer{},
		NewWorker:            s.newWorker,
	}
	s.manifold = raftforwarder.Manifold(s.config)

}

func (s *manifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"raft": s.raft,
		"hub":  s.hub,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *manifoldSuite) newWorker(config raftforwarder.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *manifoldSuite) TestValidate(c *gc.C) {
	c.Assert(s.config.Validate(), jc.ErrorIsNil)
	type test struct {
		f      func(cfg *raftforwarder.ManifoldConfig)
		expect string
	}
	tests := []test{{
		func(cfg *raftforwarder.ManifoldConfig) { cfg.CentralHubName = "" },
		"empty CentralHubName not valid",
	}, {
		func(cfg *raftforwarder.ManifoldConfig) { cfg.RequestTopic = "" },
		"empty RequestTopic not valid",
	}, {
		func(cfg *raftforwarder.ManifoldConfig) { cfg.Logger = nil },
		"nil Logger not valid",
	}, {
		func(cfg *raftforwarder.ManifoldConfig) { cfg.PrometheusRegisterer = nil },
		"nil PrometheusRegisterer not valid",
	}, {
		func(cfg *raftforwarder.ManifoldConfig) { cfg.NewWorker = nil },
		"nil NewWorker not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		// Local copy before mutating.
		cfg := s.config
		test.f(&cfg)
		c.Assert(cfg.Validate(), gc.ErrorMatches, test.expect)
	}
}

var expectedInputs = []string{
	"raft", "hub",
}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	_, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "NewWorker")

	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, raftforwarder.Config{})
	config := args[0].(raftforwarder.Config)

	c.Assert(config, jc.DeepEquals, raftforwarder.Config{
		Raft:                 s.raft,
		Hub:                  s.hub,
		Logger:               &s.logger,
		Topic:                "test.request",
		PrometheusRegisterer: s.config.PrometheusRegisterer,
	})
}

type mockWorker struct{}

func (w *mockWorker) Kill() {}
func (w *mockWorker) Wait() error {
	return nil
}

type noopRegisterer struct {
	prometheus.Registerer
}

func (noopRegisterer) Register(prometheus.Collector) error {
	return nil
}

func (noopRegisterer) Unregister(prometheus.Collector) bool {
	return true
}
