// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"context"
	"time"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/agent"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher/watchertest"
	ttesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type agentSuite struct {
	testing.ApiServerSuite

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer

	machine0  *state.Machine
	machine1  *state.Machine
	container *state.Machine

	store objectstore.ObjectStore
}

var _ = gc.Suite(&agentSuite{})

func (s *agentSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	st := s.ControllerModel(c).State()
	var err error
	s.machine0, err = st.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)

	s.machine1, err = st.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	s.container, err = st.AddMachineInsideMachine(template, s.machine1.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)

	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming machine 1 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.machine1.Tag(),
	}

	s.store = testing.NewObjectStore(c, s.ControllerModelUUID())
}

func (s *agentSuite) agentAPI(c *gc.C, auth facade.Authorizer, credentialService agent.CredentialService) *agent.AgentAPI {
	modelUUID := modeltesting.GenModelUUID(c)
	return agent.NewAgentAPI(
		auth,
		common.AuthFuncForTag(names.NewModelTag(modelUUID.String())),
		s.resources,
		s.ControllerModel(c).State(),
		nil,
		nil,
		nil,
		credentialService,
		nil,
		nil,
		nil,
		nil,
	)
}

func (s *agentSuite) TestAgentFailsWithNonAgent(c *gc.C) {
	auth := s.authorizer
	auth.Tag = names.NewUserTag("admin")
	ctx := facadetest.ModelContext{
		Auth_: auth,
	}
	_, err := agent.NewAgentAPIV3(ctx)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *agentSuite) TestAgentSucceedsWithUnitAgent(c *gc.C) {
	auth := s.authorizer
	auth.Tag = names.NewUnitTag("foosball/1")
	s.agentAPI(c, auth, nil)
}

func (s *agentSuite) TestGetEntities(c *gc.C) {
	err := s.container.Destroy(s.store)
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-0"},
			{Tag: "machine-1-lxd-0"},
			{Tag: "machine-42"},
		},
	}
	api := s.agentAPI(c, s.authorizer, nil)
	results := api.GetEntities(context.Background(), args)
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
	err := s.container.Destroy(s.store)
	c.Assert(err, jc.ErrorIsNil)

	api := s.agentAPI(c, auth, nil)
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-0"},
			{Tag: "machine-1-lxd-0"},
			{Tag: "machine-42"},
		},
	}
	results := api.GetEntities(context.Background(), args)
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
	err := s.container.Destroy(s.store)
	c.Assert(err, jc.ErrorIsNil)
	err = s.container.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.container.Remove()
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine1.Destroy(s.store)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine1.Remove()
	c.Assert(err, jc.ErrorIsNil)

	api := s.agentAPI(c, s.authorizer, nil)
	results := api.GetEntities(context.Background(), params.Entities{
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
	api := s.agentAPI(c, s.authorizer, nil)
	results, err := api.SetPasswords(
		context.Background(),
		params.EntityPasswords{
			Changes: []params.EntityPassword{
				{Tag: "machine-0", Password: "xxx-12345678901234567890"},
				{Tag: "machine-1", Password: "yyy-12345678901234567890"},
				{Tag: "machine-42", Password: "zzz-12345678901234567890"},
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	changed := s.machine1.PasswordValid("yyy-12345678901234567890")
	c.Assert(changed, jc.IsTrue)
}

func (s *agentSuite) TestSetPasswordsShort(c *gc.C) {
	api := s.agentAPI(c, s.authorizer, nil)
	results, err := api.SetPasswords(
		context.Background(),
		params.EntityPasswords{
			Changes: []params.EntityPassword{
				{Tag: "machine-1", Password: "yyy"},
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"password is only 3 bytes long, and is not a valid Agent password")
}

func (s *agentSuite) TestWatchCredentials(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	tag := names.NewCloudCredentialTag("dummy/fred/default")

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	credentialService := NewMockCredentialService(ctrl)

	ch := make(chan struct{}, 1)
	// Initial event.
	ch <- struct{}{}

	nw := watchertest.NewMockNotifyWatcher(ch)
	credentialService.EXPECT().WatchCredential(gomock.Any(), credential.KeyFromTag(tag)).Return(nw, nil)
	api := s.agentAPI(c, authorizer, credentialService)
	result, err := api.WatchCredentials(context.Background(), params.Entities{Entities: []params.Entity{{Tag: tag.String()}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{NotifyWatcherId: "1", Error: nil}}})
	c.Assert(s.resources.Count(), gc.Equals, 1)

	w := s.resources.Get("1")
	defer workertest.CleanKill(c, w)

	// Check that the Watch has consumed the initial events ("returned" in the Watch call)
	wc := watchertest.NewNotifyWatcherC(c, w.(state.NotifyWatcher))
	wc.AssertNoChange()

	select {
	case ch <- struct{}{}:
	case <-time.After(ttesting.LongWait):
		c.Fatalf("timeout sending credential change event")
	}
	wc.AssertOneChange()
}

func (s *agentSuite) TestWatchAuthError(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("1"),
		Controller: false,
	}
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	credentialService := NewMockCredentialService(ctrl)

	api := s.agentAPI(c, authorizer, credentialService)
	_, err := api.WatchCredentials(context.Background(), params.Entities{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(s.resources.Count(), gc.Equals, 0)
}
