package agent_test

import (
	stdtesting "testing"

	"github.com/juju/names"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/agent"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type apiConstructor func(st *state.State, resources *common.Resources, auth common.Authorizer) (interface{}, error)

// agentAPIV0 helps decoupling the static type of agent API V0.
type agentAPIV0 interface {
	GetEntities(args params.Entities) params.AgentGetEntitiesResults
	StateServingInfo() (state.StateServingInfo, error)
	IsMaster() (params.IsMasterResult, error)
	SetPasswords(args params.EntityPasswords) (params.ErrorResults, error)
}

func newAgentAPIV0(st *state.State, resources *common.Resources, auth common.Authorizer) (interface{}, error) {
	return agent.NewAgentAPIV0(st, resources, auth)
}

// agentAPIV1 is compatible with V0.
type agentAPIV1 interface {
	agentAPIV0
}

func newAgentAPIV1(st *state.State, resources *common.Resources, auth common.Authorizer) (interface{}, error) {
	return agent.NewAgentAPIV1(st, resources, auth)
}

type baseAgentSuite struct {
	jujutesting.JujuConnSuite

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer

	machine0    *state.Machine
	machine1    *state.Machine
	container   *state.Machine
	constructor apiConstructor
}

func (s *baseAgentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	var err error
	s.machine0, err = s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)

	s.machine1, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	s.container, err = s.State.AddMachineInsideMachine(template, s.machine1.Id(), instance.LXC)
	c.Assert(err, gc.IsNil)

	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming machine 1 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.machine1.Tag(),
	}
}

// authorizationAgentSuiteV0 tests authorization from V0
// up to the next incompatible change.
type authorizationAgentSuiteV0 struct {
	baseAgentSuite
}

func (s *authorizationAgentSuiteV0) TestAgentFailsWithNonAgent(c *gc.C) {
	auth := s.authorizer
	auth.Tag = names.NewUserTag("admin")
	api, err := s.constructor(s.State, s.resources, auth)
	c.Assert(err, gc.NotNil)
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *authorizationAgentSuiteV0) TestAgentSucceedsWithUnitAgent(c *gc.C) {
	auth := s.authorizer
	auth.Tag = names.NewUnitTag("foosball/1")
	_, err := s.constructor(s.State, s.resources, auth)
	c.Assert(err, gc.IsNil)
}

// getEntitiesAgentSuiteV0 tests GetEntities from V0
// up to the next incompatible change.
type getEntitiesAgentSuiteV0 struct {
	baseAgentSuite
}

func (s *getEntitiesAgentSuiteV0) TestGetEntities(c *gc.C) {
	api, err := s.constructor(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	agentAPI := api.(agentAPIV0)
	err = s.container.Destroy()
	c.Assert(err, gc.IsNil)
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-0"},
			{Tag: "machine-1-lxc-0"},
			{Tag: "machine-42"},
		},
	}
	results := agentAPI.GetEntities(args)
	c.Assert(results, gc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{
			{
				Life: "alive",
				Jobs: []params.MachineJob{params.JobHostUnits},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now login as the machine agent of the container and try again.
	auth := s.authorizer
	auth.Tag = s.container.Tag()
	api, err = s.constructor(s.State, s.resources, auth)
	c.Assert(err, gc.IsNil)
	containerAgent := api.(agentAPIV0)

	results = containerAgent.GetEntities(args)
	c.Assert(results, gc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{
				Life:          "dying",
				Jobs:          []params.MachineJob{params.JobHostUnits},
				ContainerType: instance.LXC,
			},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *getEntitiesAgentSuiteV0) TestGetNotFoundEntity(c *gc.C) {
	api, err := s.constructor(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	agentAPI := api.(agentAPIV0)
	// Destroy the container first, so we can destroy its parent.
	err = s.container.Destroy()
	c.Assert(err, gc.IsNil)
	err = s.container.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.container.Remove()
	c.Assert(err, gc.IsNil)

	err = s.machine1.Destroy()
	c.Assert(err, gc.IsNil)
	err = s.machine1.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.machine1.Remove()
	c.Assert(err, gc.IsNil)
	results := agentAPI.GetEntities(params.Entities{
		Entities: []params.Entity{{Tag: "machine-1"}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{{
			Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "machine 1 not found",
			},
		}},
	})
}

// setPasswordsAgentSuiteV0 tests SetPasswords from V0
// up to the next incompatible change.
type setPasswordsAgentSuiteV0 struct {
	baseAgentSuite
}

func (s *setPasswordsAgentSuiteV0) TestSetPasswords(c *gc.C) {
	api, err := s.constructor(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	agentAPI := api.(agentAPIV0)
	results, err := agentAPI.SetPasswords(params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "machine-0", Password: "xxx-12345678901234567890"},
			{Tag: "machine-1", Password: "yyy-12345678901234567890"},
			{Tag: "machine-42", Password: "zzz-12345678901234567890"},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})
	err = s.machine1.Refresh()
	c.Assert(err, gc.IsNil)
	changed := s.machine1.PasswordValid("yyy-12345678901234567890")
	c.Assert(changed, gc.Equals, true)
}

func (s *setPasswordsAgentSuiteV0) TestShortSetPasswords(c *gc.C) {
	api, err := s.constructor(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	agentAPI := api.(agentAPIV0)
	results, err := agentAPI.SetPasswords(params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "machine-1", Password: "yyy"},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"password is only 3 bytes long, and is not a valid Agent password")
}

var (
	_ = gc.Suite(&authorizationAgentSuiteV0{
		baseAgentSuite{
			constructor: newAgentAPIV0,
		},
	})
	_ = gc.Suite(&authorizationAgentSuiteV0{
		baseAgentSuite{
			constructor: newAgentAPIV1,
		},
	})

	_ = gc.Suite(&getEntitiesAgentSuiteV0{
		baseAgentSuite{
			constructor: newAgentAPIV0,
		},
	})
	_ = gc.Suite(&getEntitiesAgentSuiteV0{
		baseAgentSuite{
			constructor: newAgentAPIV1,
		},
	})

	_ = gc.Suite(&setPasswordsAgentSuiteV0{
		baseAgentSuite{
			constructor: newAgentAPIV0,
		},
	})
	_ = gc.Suite(&setPasswordsAgentSuiteV0{
		baseAgentSuite{
			constructor: newAgentAPIV1,
		},
	})
)
