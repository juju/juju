// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer_test

import (
	"errors"
	"time"

	"github.com/juju/clock"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	coretesting "github.com/juju/juju/testing"
	resumer "github.com/juju/juju/worker/resumer"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (*ManifoldSuite) TestInputs(c *gc.C) {
	manifold := resumer.Manifold(resumer.ManifoldConfig{
		AgentName:     "bill",
		APICallerName: "ben",
	})
	expect := []string{"bill", "ben"}
	c.Check(manifold.Inputs, jc.DeepEquals, expect)
}

func (*ManifoldSuite) TestOutput(c *gc.C) {
	manifold := resumer.Manifold(resumer.ManifoldConfig{})
	c.Check(manifold.Output, gc.IsNil)
}

func (*ManifoldSuite) TestMissingAgent(c *gc.C) {
	manifold := resumer.Manifold(resumer.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	})

	worker, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":      dependency.ErrMissing,
		"api-caller": &fakeAPICaller{},
	}))
	workertest.CheckNilOrKill(c, worker)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestMissingAPICaller(c *gc.C) {
	manifold := resumer.Manifold(resumer.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	})

	worker, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":      &fakeAgent{},
		"api-caller": dependency.ErrMissing,
	}))
	workertest.CheckNilOrKill(c, worker)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestAgentEntity_Error(c *gc.C) {
	manifold := resumer.Manifold(resumer.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	})

	stub := &testing.Stub{}
	stub.SetErrors(errors.New("zap"))
	apiCaller := &fakeAPICaller{stub: stub}
	worker, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":      &fakeAgent{},
		"api-caller": apiCaller,
	}))
	workertest.CheckNilOrKill(c, worker)
	c.Check(err, gc.ErrorMatches, "zap")

	stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Agent.GetEntities",
		Args: []interface{}{params.Entities{
			Entities: []params.Entity{{
				Tag: "machine-123",
			}},
		}},
	}})
}

func (s *ManifoldSuite) TestAgentEntity_NoJob(c *gc.C) {
	manifold := resumer.Manifold(resumer.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	})

	worker, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":      &fakeAgent{},
		"api-caller": &fakeAPICaller{},
	}))
	workertest.CheckNilOrKill(c, worker)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestAgentEntity_NotModelManager(c *gc.C) {
	manifold := resumer.Manifold(resumer.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	})

	worker, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":      &fakeAgent{},
		"api-caller": newFakeAPICaller(model.JobHostUnits),
	}))
	workertest.CheckNilOrKill(c, worker)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestNewFacade_Missing(c *gc.C) {
	manifold := resumer.Manifold(resumer.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	})

	worker, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":      &fakeAgent{},
		"api-caller": newFakeAPICaller(model.JobManageModel),
	}))
	workertest.CheckNilOrKill(c, worker)
	c.Check(err, gc.Equals, dependency.ErrUninstall)
}

func (s *ManifoldSuite) TestNewFacade_Error(c *gc.C) {
	apiCaller := newFakeAPICaller(model.JobManageModel)
	manifold := resumer.Manifold(resumer.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		NewFacade: func(actual base.APICaller) (resumer.Facade, error) {
			c.Check(actual, gc.Equals, apiCaller)
			return nil, errors.New("pow")
		},
	})

	worker, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":      &fakeAgent{},
		"api-caller": apiCaller,
	}))
	workertest.CheckNilOrKill(c, worker)
	c.Check(err, gc.ErrorMatches, "pow")
}

func (s *ManifoldSuite) TestNewWorker_Missing(c *gc.C) {
	manifold := resumer.Manifold(resumer.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		NewFacade: func(base.APICaller) (resumer.Facade, error) {
			return &fakeFacade{}, nil
		},
	})

	worker, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":      &fakeAgent{},
		"api-caller": newFakeAPICaller(model.JobManageModel),
	}))
	workertest.CheckNilOrKill(c, worker)
	c.Check(err, gc.Equals, dependency.ErrUninstall)
}

func (s *ManifoldSuite) TestNewWorker_Error(c *gc.C) {
	clock := &fakeClock{}
	facade := &fakeFacade{}
	manifold := resumer.Manifold(resumer.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		Clock:         clock,
		Interval:      time.Hour,
		NewFacade: func(base.APICaller) (resumer.Facade, error) {
			return facade, nil
		},
		NewWorker: func(actual resumer.Config) (worker.Worker, error) {
			c.Check(actual, jc.DeepEquals, resumer.Config{
				Facade:   facade,
				Clock:    clock,
				Interval: time.Hour,
			})
			return nil, errors.New("blam")
		},
	})

	worker, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":      &fakeAgent{},
		"api-caller": newFakeAPICaller(model.JobManageModel),
	}))
	workertest.CheckNilOrKill(c, worker)
	c.Check(err, gc.ErrorMatches, "blam")
}

func (s *ManifoldSuite) TestNewWorker_Success(c *gc.C) {
	expect := &fakeWorker{}
	manifold := resumer.Manifold(resumer.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		NewFacade: func(base.APICaller) (resumer.Facade, error) {
			return &fakeFacade{}, nil
		},
		NewWorker: func(actual resumer.Config) (worker.Worker, error) {
			return expect, nil
		},
	})

	actual, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":      &fakeAgent{},
		"api-caller": newFakeAPICaller(model.JobManageModel),
	}))
	c.Check(err, jc.ErrorIsNil)
	c.Check(actual, gc.Equals, expect)
}

// fakeFacade should not be called.
type fakeFacade struct {
	resumer.Facade
}

// fakeClock should not be called.
type fakeClock struct {
	clock.Clock
}

// fakeWorker should not be called.
type fakeWorker struct {
	worker.Worker
}

// fakeAgent exists to expose a tag via CurrentConfig().Tag().
type fakeAgent struct {
	agent.Agent
}

// CurrentConfig returns an agent.Config with a working Tag() method.
func (a *fakeAgent) CurrentConfig() agent.Config {
	return &fakeConfig{}
}

// fakeConfig exists to expose Tag.
type fakeConfig struct {
	agent.Config
}

// Tag returns a Tag.
func (c *fakeConfig) Tag() names.Tag {
	return names.NewMachineTag("123")
}

func newFakeAPICaller(jobs ...model.MachineJob) *fakeAPICaller {
	return &fakeAPICaller{jobs: jobs}
}

// fakeAPICaller exists to handle the hackish checkModelManager's api
// call directly, because it shouldn't happen in this context at all
// and we don't want it leaking into the config.
type fakeAPICaller struct {
	base.APICaller
	stub *testing.Stub
	jobs []model.MachineJob
}

// APICall is part of the base.APICaller interface.
func (f *fakeAPICaller) APICall(objType string, version int, id, request string, args interface{}, response interface{}) error {
	if f.stub != nil {
		// We don't usually set the stub here, most of the time
		// the APICall hack is just an unwanted distraction from
		// the NewFacade/NewWorker bits that *should* exist long-
		// term. This makes it easier to just delete the broken
		// tests, and most of this type, including all of the
		// methods, when we drop the job check.
		f.stub.AddCall(objType+"."+request, args)
		if err := f.stub.NextErr(); err != nil {
			return err
		}
	}

	if res, ok := response.(*params.AgentGetEntitiesResults); ok {
		jobs := make([]model.MachineJob, 0, len(f.jobs))
		jobs = append(jobs, f.jobs...)
		res.Entities = []params.AgentGetEntitiesResult{
			{Jobs: jobs},
		}
	}
	return nil
}

// BestFacadeVersion is part of the base.APICaller interface.
func (*fakeAPICaller) BestFacadeVersion(facade string) int {
	return 42
}

// ModelTag is part of the base.APICaller interface.
func (*fakeAPICaller) ModelTag() (names.ModelTag, bool) {
	return coretesting.ModelTag, true
}
