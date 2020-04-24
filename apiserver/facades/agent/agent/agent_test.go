// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	stdtesting "testing"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/agent"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

var _ = gc.Suite(&agentSuite{})

type agentSuite struct {
	jujutesting.JujuConnSuite

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer

	machine0  *state.Machine
	machine1  *state.Machine
	container *state.Machine
}

func (s *agentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	var err error
	s.machine0, err = s.State.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)

	s.machine1, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	s.container, err = s.State.AddMachineInsideMachine(template, s.machine1.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)

	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming machine 1 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.machine1.Tag(),
	}
}

func (s *agentSuite) TestAgentFailsWithNonAgent(c *gc.C) {
	auth := s.authorizer
	auth.Tag = names.NewUserTag("admin")
	api, err := agent.NewAgentAPIV2(s.State, s.resources, auth)
	c.Assert(err, gc.NotNil)
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *agentSuite) TestAgentSucceedsWithUnitAgent(c *gc.C) {
	auth := s.authorizer
	auth.Tag = names.NewUnitTag("foosball/1")
	_, err := agent.NewAgentAPIV2(s.State, s.resources, auth)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *agentSuite) TestGetEntities(c *gc.C) {
	err := s.container.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-0"},
			{Tag: "machine-1-lxd-0"},
			{Tag: "machine-42"},
		},
	}
	api, err := agent.NewAgentAPIV2(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	results := api.GetEntities(args)
	c.Assert(results, gc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{
			{
				Life: "alive",
				Jobs: []model.MachineJob{model.JobHostUnits},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *agentSuite) TestGetEntitiesContainer(c *gc.C) {
	auth := s.authorizer
	auth.Tag = s.container.Tag()
	err := s.container.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	api, err := agent.NewAgentAPIV2(s.State, s.resources, auth)
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-0"},
			{Tag: "machine-1-lxd-0"},
			{Tag: "machine-42"},
		},
	}
	results := api.GetEntities(args)
	c.Assert(results, gc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{
				Life:          "dying",
				Jobs:          []model.MachineJob{model.JobHostUnits},
				ContainerType: instance.LXD,
			},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *agentSuite) TestGetEntitiesNotFound(c *gc.C) {
	// Destroy the container first, so we can destroy its parent.
	err := s.container.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.container.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.container.Remove()
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine1.Remove()
	c.Assert(err, jc.ErrorIsNil)

	api, err := agent.NewAgentAPIV2(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	results := api.GetEntities(params.Entities{
		Entities: []params.Entity{{Tag: "machine-1"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{{
			Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "machine 1 not found",
			},
		}},
	})
}

func (s *agentSuite) TestSetPasswords(c *gc.C) {
	api, err := agent.NewAgentAPIV2(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	results, err := api.SetPasswords(params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "machine-0", Password: "xxx-12345678901234567890"},
			{Tag: "machine-1", Password: "yyy-12345678901234567890"},
			{Tag: "machine-42", Password: "zzz-12345678901234567890"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})
	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	changed := s.machine1.PasswordValid("yyy-12345678901234567890")
	c.Assert(changed, jc.IsTrue)
}

func (s *agentSuite) TestSetPasswordsShort(c *gc.C) {
	api, err := agent.NewAgentAPIV2(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	results, err := api.SetPasswords(params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "machine-1", Password: "yyy"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"password is only 3 bytes long, and is not a valid Agent password")
}

func (s *agentSuite) TestClearReboot(c *gc.C) {
	api, err := agent.NewAgentAPIV2(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine1.SetRebootFlag(true)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machine0.Tag().String()},
		{Tag: s.machine1.Tag().String()},
	}}

	rFlag, err := s.machine1.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rFlag, jc.IsTrue)

	result, err := api.ClearReboot(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
		},
	})

	rFlag, err = s.machine1.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rFlag, jc.IsFalse)
}

func (s *agentSuite) TestWatchCredentials(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	api, err := agent.NewAgentAPIV2(s.State, s.resources, authorizer)
	c.Assert(err, jc.ErrorIsNil)
	tag := names.NewCloudCredentialTag("dummy/fred/default")
	result, err := api.WatchCredentials(params.Entities{Entities: []params.Entity{{Tag: tag.String()}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{"1", nil}}})
	c.Assert(s.resources.Count(), gc.Equals, 1)

	w := s.resources.Get("1")
	defer statetesting.AssertStop(c, w)

	// Check that the Watch has consumed the initial events ("returned" in the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w.(state.NotifyWatcher))
	wc.AssertNoChange()

	s.State.UpdateCloudCredential(tag, cloud.NewCredential(cloud.UserPassAuthType, nil))
	wc.AssertOneChange()
}

func (s *agentSuite) TestWatchAuthError(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("1"),
		Controller: false,
	}
	api, err := agent.NewAgentAPIV2(s.State, s.resources, authorizer)
	c.Assert(err, jc.ErrorIsNil)
	_, err = api.WatchCredentials(params.Entities{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(s.resources.Count(), gc.Equals, 0)
}
