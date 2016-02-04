// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreagent "github.com/juju/juju/agent"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
)

type ServingInfoSetterSuite struct {
	testing.BaseSuite
	manifold dependency.Manifold
}

var _ = gc.Suite(&ServingInfoSetterSuite{})

func (s *ServingInfoSetterSuite) SetUpTest(c *gc.C) {
	s.manifold = machine.ServingInfoSetterManifold(machine.ServingInfoSetterConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	})
}

func (s *ServingInfoSetterSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, []string{
		"agent",
		"api-caller",
	})
}

func (s *ServingInfoSetterSuite) TestStartAgentMissing(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent": dt.StubResource{Error: dependency.ErrMissing},
	})
	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ServingInfoSetterSuite) TestStartApiCallerMissing(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent":      dt.StubResource{Output: &mockAgent{}},
		"api-caller": dt.StubResource{Error: dependency.ErrMissing},
	})
	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ServingInfoSetterSuite) TestNotMachine(c *gc.C) {
	a := &mockAgent{
		conf: mockConfig{tag: names.NewUnitTag("foo/0")},
	}
	getResource := dt.StubGetResource(dt.StubResources{
		"agent": dt.StubResource{Output: a},
	})
	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "agent's tag is not a machine tag")
}

func (s *ServingInfoSetterSuite) TestEntityLookupFailure(c *gc.C) {
	// Set up a fake Agent and APICaller
	a := &mockAgent{}
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			c.Assert(objType, gc.Equals, "Agent")
			switch request {
			case "GetEntities":
				c.Assert(args.(params.Entities).Entities, gc.HasLen, 1)
				result := response.(*params.AgentGetEntitiesResults)
				result.Entities = []params.AgentGetEntitiesResult{{
					Error: &params.Error{Message: "boom"},
				}}
			default:
				c.Fatalf("not sure how to handle: %q", request)
			}
			return nil
		},
	)
	// Call the manifold's start func with a fake resource getter that
	// returns the fake Agent and APICaller
	w, err := s.manifold.Start(dt.StubGetResource(dt.StubResources{
		"agent":      dt.StubResource{Output: a},
		"api-caller": dt.StubResource{Output: apiCaller},
	}))
	c.Assert(w, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ServingInfoSetterSuite) TestJobManageEnviron(c *gc.C) {
	// State serving info should be set for machines with JobManageEnviron.
	const mockAPIPort = 1234

	a := &mockAgent{}
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			c.Assert(objType, gc.Equals, "Agent")
			switch request {
			case "GetEntities":
				c.Assert(args.(params.Entities).Entities, gc.HasLen, 1)
				result := response.(*params.AgentGetEntitiesResults)
				result.Entities = []params.AgentGetEntitiesResult{{
					Jobs: []multiwatcher.MachineJob{multiwatcher.JobManageModel},
				}}
			case "StateServingInfo":
				result := response.(*params.StateServingInfo)
				*result = params.StateServingInfo{
					APIPort: mockAPIPort,
				}
			default:
				c.Fatalf("not sure how to handle: %q", request)
			}
			return nil
		},
	)
	w, err := s.manifold.Start(dt.StubGetResource(dt.StubResources{
		"agent":      dt.StubResource{Output: a},
		"api-caller": dt.StubResource{Output: apiCaller},
	}))
	c.Assert(w, gc.IsNil)
	c.Assert(err, gc.Equals, dependency.ErrUninstall)

	// Verify that the state serving info was actually set.
	c.Assert(a.conf.ssiSet, jc.IsTrue)
	c.Assert(a.conf.ssi.APIPort, gc.Equals, mockAPIPort)
}

func (s *ServingInfoSetterSuite) TestJobHostUnits(c *gc.C) {
	// State serving info should not be set for JobHostUnits.
	s.checkNotController(c, multiwatcher.JobHostUnits)
}

func (s *ServingInfoSetterSuite) TestJobManageNetworking(c *gc.C) {
	// State serving info should NOT be set for JobManageNetworking.
	s.checkNotController(c, multiwatcher.JobManageNetworking)
}

func (s *ServingInfoSetterSuite) checkNotController(c *gc.C, job multiwatcher.MachineJob) {
	a := &mockAgent{}
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			c.Assert(objType, gc.Equals, "Agent")
			switch request {
			case "GetEntities":
				c.Assert(args.(params.Entities).Entities, gc.HasLen, 1)
				result := response.(*params.AgentGetEntitiesResults)
				result.Entities = []params.AgentGetEntitiesResult{{
					Jobs: []multiwatcher.MachineJob{job},
				}}
			default:
				c.Fatalf("not sure how to handle: %q", request)
			}
			return nil
		},
	)
	w, err := s.manifold.Start(dt.StubGetResource(dt.StubResources{
		"agent":      dt.StubResource{Output: a},
		"api-caller": dt.StubResource{Output: apiCaller},
	}))
	c.Assert(w, gc.IsNil)
	c.Assert(err, gc.Equals, dependency.ErrUninstall)

	// State serving info shouldn't have been set for this job type.
	c.Assert(a.conf.ssiSet, jc.IsFalse)
}

type mockAgent struct {
	coreagent.Agent
	conf mockConfig
}

func (ma *mockAgent) CurrentConfig() coreagent.Config {
	return &ma.conf
}

func (ma *mockAgent) ChangeConfig(f coreagent.ConfigMutator) error {
	return f(&ma.conf)
}

type mockConfig struct {
	coreagent.ConfigSetter
	tag    names.Tag
	ssiSet bool
	ssi    params.StateServingInfo
}

func (mc *mockConfig) Tag() names.Tag {
	if mc.tag == nil {
		return names.NewMachineTag("99")
	}
	return mc.tag
}

func (mc *mockConfig) SetStateServingInfo(info params.StateServingInfo) {
	mc.ssiSet = true
	mc.ssi = info
}
