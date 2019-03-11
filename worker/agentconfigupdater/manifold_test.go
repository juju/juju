// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1/dependency"
	dt "gopkg.in/juju/worker.v1/dependency/testing"

	"github.com/juju/juju/agent"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/agentconfigupdater"
)

type AgentConfigUpdaterSuite struct {
	testing.BaseSuite
	manifold dependency.Manifold
}

var _ = gc.Suite(&AgentConfigUpdaterSuite{})

func (s *AgentConfigUpdaterSuite) SetUpTest(c *gc.C) {
	s.manifold = agentconfigupdater.Manifold(agentconfigupdater.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	})
}

func (s *AgentConfigUpdaterSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, []string{
		"agent",
		"api-caller",
	})
}

func (s *AgentConfigUpdaterSuite) TestStartAgentMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent": dependency.ErrMissing,
	})
	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *AgentConfigUpdaterSuite) TestStartAPICallerMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent":      &mockAgent{},
		"api-caller": dependency.ErrMissing,
	})
	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *AgentConfigUpdaterSuite) TestNotMachine(c *gc.C) {
	a := &mockAgent{
		conf: mockConfig{tag: names.NewUnitTag("foo/0")},
	}
	context := dt.StubContext(nil, map[string]interface{}{
		"agent": a,
	})
	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "agent's tag is not a machine tag")
}

func (s *AgentConfigUpdaterSuite) TestEntityLookupFailure(c *gc.C) {
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
	context := dt.StubContext(nil, map[string]interface{}{
		"agent":      a,
		"api-caller": apiCaller,
	})
	w, err := s.manifold.Start(context)
	c.Assert(w, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "checking controller status: boom")
}

func (s *AgentConfigUpdaterSuite) startManifold(c *gc.C, a agent.Agent, mockAPIPort int) {
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
					Cert:       "cert",
					PrivateKey: "key",
					APIPort:    mockAPIPort,
				}
			default:
				c.Fatalf("not sure how to handle: %q", request)
			}
			return nil
		},
	)
	context := dt.StubContext(nil, map[string]interface{}{
		"agent":      a,
		"api-caller": apiCaller,
	})
	w, err := s.manifold.Start(context)
	c.Assert(w, gc.IsNil)
	c.Assert(err, gc.Equals, dependency.ErrUninstall)
}

func (s *AgentConfigUpdaterSuite) TestJobManageEnviron(c *gc.C) {
	// State serving info should be set for machines with JobManageEnviron.
	const mockAPIPort = 1234

	a := &mockAgent{}
	s.startManifold(c, a, mockAPIPort)

	// Verify that the state serving info was actually set.
	c.Assert(a.conf.ssiSet, jc.IsTrue)
	c.Assert(a.conf.ssi.APIPort, gc.Equals, mockAPIPort)
	c.Assert(a.conf.ssi.Cert, gc.Equals, "cert")
	c.Assert(a.conf.ssi.PrivateKey, gc.Equals, "key")
}

func (s *AgentConfigUpdaterSuite) TestJobManageEnvironNotOverwriteCert(c *gc.C) {
	// State serving info should be set for machines with JobManageEnviron.
	const mockAPIPort = 1234

	a := &mockAgent{}
	existingCert := "some cert set by certupdater"
	existingKey := "some key set by certupdater"
	a.conf.SetStateServingInfo(params.StateServingInfo{
		Cert:       existingCert,
		PrivateKey: existingKey,
	})

	s.startManifold(c, a, mockAPIPort)

	// Verify that the state serving info was actually set.
	c.Assert(a.conf.ssiSet, jc.IsTrue)
	c.Assert(a.conf.ssi.APIPort, gc.Equals, mockAPIPort)
	c.Assert(a.conf.ssi.Cert, gc.Equals, existingCert)
	c.Assert(a.conf.ssi.PrivateKey, gc.Equals, existingKey)
}

func (s *AgentConfigUpdaterSuite) TestJobHostUnits(c *gc.C) {
	// State serving info should not be set for JobHostUnits.
	s.checkNotController(c, multiwatcher.JobHostUnits)
}

func (s *AgentConfigUpdaterSuite) checkNotController(c *gc.C, job multiwatcher.MachineJob) {
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
	w, err := s.manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":      a,
		"api-caller": apiCaller,
	}))
	c.Assert(w, gc.IsNil)
	c.Assert(err, gc.Equals, dependency.ErrUninstall)

	// State serving info shouldn't have been set for this job type.
	c.Assert(a.conf.ssiSet, jc.IsFalse)
}

type mockAgent struct {
	agent.Agent
	conf mockConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

func (ma *mockAgent) ChangeConfig(f agent.ConfigMutator) error {
	return f(&ma.conf)
}

type mockConfig struct {
	agent.ConfigSetter
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

func (mc *mockConfig) Controller() names.ControllerTag {
	return testing.ControllerTag
}

func (mc *mockConfig) StateServingInfo() (params.StateServingInfo, bool) {
	return mc.ssi, mc.ssiSet
}

func (mc *mockConfig) SetStateServingInfo(info params.StateServingInfo) {
	mc.ssiSet = true
	mc.ssi = info
}

func (mc *mockConfig) LogDir() string {
	return "log-dir"
}
