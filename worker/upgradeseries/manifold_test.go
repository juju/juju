// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package upgradeseries_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/upgradeseries"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold dependency.Manifold
	context  dependency.Context
	worker   worker.Worker
	stub     testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub.ResetCalls()

	type mockWorker struct {
		worker.Worker
	}
	s.worker = &mockWorker{}

	s.context = s.newContext(nil)
	s.manifold = upgradeseries.Manifold(upgradeseries.ManifoldConfig{
		NewWorker: s.newWorker,
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		// ???
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(config upgradeseries.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (*ManifoldSuite) TestOutputBadWorker(c *gc.C) {
	manifold := upgradeseries.Manifold(upgradeseries.ManifoldConfig{})
	in := &struct{ worker.Worker }{}
	var out engine.Flag
	err := manifold.Output(in, &out)
	c.Check(err, gc.ErrorMatches, "expected in to implement Flag; got a .*")
}

func (*ManifoldSuite) TestStartMissingNewFacade(c *gc.C) {
	config := validManifoldConfig()
	config.NewFacade = nil
	checkManifoldNotValid(c, config, "nil NewFacade not valid")
}

func (*ManifoldSuite) TestStartMissingNewWorker(c *gc.C) {
	config := validManifoldConfig()
	config.NewWorker = nil
	checkManifoldNotValid(c, config, "nil NewWorker not valid")
}
