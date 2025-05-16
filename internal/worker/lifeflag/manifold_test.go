// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/lifeflag"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *stdtesting.T) { tc.Run(t, &ManifoldSuite{}) }
func (*ManifoldSuite) TestInputs(c *tc.C) {
	manifold := lifeflag.Manifold(lifeflag.ManifoldConfig{
		APICallerName: "boris",
	})
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"boris"})
}

func (*ManifoldSuite) TestFilter(c *tc.C) {
	expect := errors.New("squish")
	manifold := lifeflag.Manifold(lifeflag.ManifoldConfig{
		Filter: func(error) error { return expect },
	})
	actual := manifold.Filter(errors.New("blarg"))
	c.Check(actual, tc.Equals, expect)
}

func (*ManifoldSuite) TestOutputBadWorker(c *tc.C) {
	manifold := lifeflag.Manifold(lifeflag.ManifoldConfig{})
	worker := struct{ worker.Worker }{}
	var flag engine.Flag
	err := manifold.Output(worker, &flag)
	c.Check(err, tc.ErrorMatches, "expected in to implement Flag; got a .*")
}

func (*ManifoldSuite) TestOutputBadTarget(c *tc.C) {
	manifold := lifeflag.Manifold(lifeflag.ManifoldConfig{})
	worker := &lifeflag.Worker{}
	var flag interface{}
	err := manifold.Output(worker, &flag)
	c.Check(err, tc.ErrorMatches, "expected out to be a \\*Flag; got a .*")
}

func (*ManifoldSuite) TestOutputSuccess(c *tc.C) {
	manifold := lifeflag.Manifold(lifeflag.ManifoldConfig{})
	worker := &lifeflag.Worker{}
	var flag engine.Flag
	err := manifold.Output(worker, &flag)
	c.Check(err, tc.ErrorIsNil)
	c.Check(flag, tc.Equals, worker)
}

func (*ManifoldSuite) TestMissingAPICaller(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"api-caller": dependency.ErrMissing,
	})
	manifold := lifeflag.Manifold(lifeflag.ManifoldConfig{
		APICallerName: "api-caller",
	})

	worker, err := manifold.Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestNewWorkerError(c *tc.C) {
	expectFacade := struct{ lifeflag.Facade }{}
	expectEntity := names.NewMachineTag("33")
	getter := dt.StubGetter(map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
	})
	manifold := lifeflag.Manifold(lifeflag.ManifoldConfig{
		APICallerName: "api-caller",
		Entity:        expectEntity,
		Result:        life.IsNotAlive,
		NewFacade: func(_ base.APICaller) (lifeflag.Facade, error) {
			return expectFacade, nil
		},
		NewWorker: func(_ context.Context, config lifeflag.Config) (worker.Worker, error) {
			c.Check(config.Facade, tc.Equals, expectFacade)
			c.Check(config.Entity, tc.Equals, expectEntity)
			c.Check(config.Result, tc.NotNil) // uncomparable
			return nil, errors.New("boof")
		},
	})

	worker, err := manifold.Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "boof")
}

func (*ManifoldSuite) TestNewWorkerSuccess(c *tc.C) {
	expectWorker := &struct{ worker.Worker }{}
	getter := dt.StubGetter(map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
	})
	manifold := lifeflag.Manifold(lifeflag.ManifoldConfig{
		APICallerName: "api-caller",
		NewFacade: func(_ base.APICaller) (lifeflag.Facade, error) {
			return struct{ lifeflag.Facade }{}, nil
		},
		NewWorker: func(_ context.Context, _ lifeflag.Config) (worker.Worker, error) {
			return expectWorker, nil
		},
	})

	worker, err := manifold.Start(c.Context(), getter)
	c.Check(worker, tc.Equals, expectWorker)
	c.Check(err, tc.ErrorIsNil)
}

func (*ManifoldSuite) TestNewWorkerSuccessWithAgentName(c *tc.C) {
	expectWorker := &struct{ worker.Worker }{}
	getter := dt.StubGetter(map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"agent-name": &fakeAgent{config: fakeConfig{tag: names.NewUnitTag("ubuntu/0")}},
	})
	manifold := lifeflag.Manifold(lifeflag.ManifoldConfig{
		APICallerName: "api-caller",
		AgentName:     "agent-name",
		NewFacade: func(_ base.APICaller) (lifeflag.Facade, error) {
			return struct{ lifeflag.Facade }{}, nil
		},
		NewWorker: func(_ context.Context, config lifeflag.Config) (worker.Worker, error) {
			c.Check(config.Entity, tc.Equals, names.NewUnitTag("ubuntu/0"))
			return expectWorker, nil
		},
	})

	worker, err := manifold.Start(c.Context(), getter)
	c.Check(worker, tc.Equals, expectWorker)
	c.Check(err, tc.ErrorIsNil)
}

type fakeAgent struct {
	agent.Agent
	config fakeConfig
}

func (f *fakeAgent) CurrentConfig() agent.Config {
	return &f.config
}

type fakeConfig struct {
	agent.Config
	tag names.Tag
}

func (f *fakeConfig) Tag() names.Tag {
	return f.tag
}
