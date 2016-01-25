// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/controller"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type controllerSuite struct {
	jujutesting.JujuConnSuite

	controller *controller.ControllerAPI
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}

	controller, err := controller.NewControllerAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.controller = controller

	loggo.GetLogger("juju.apiserver.controller").SetLogLevel(loggo.TRACE)
}

func (s *controllerSuite) TestNewAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("mysql/0"),
	}
	endPoint, err := controller.NewControllerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *controllerSuite) TestNewAPIRefusesNonAdmins(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoEnvUser: true})
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: user.Tag(),
	}
	endPoint, err := controller.NewControllerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *controllerSuite) checkEnvironmentMatches(c *gc.C, env params.Model, expected *state.Model) {
	c.Check(env.Name, gc.Equals, expected.Name())
	c.Check(env.UUID, gc.Equals, expected.UUID())
	c.Check(env.OwnerTag, gc.Equals, expected.Owner().String())
}

func (s *controllerSuite) TestAllModels(c *gc.C) {
	admin := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar"})

	s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "owned", Owner: admin.UserTag()}).Close()
	remoteUserTag := names.NewUserTag("user@remote")
	st := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "user", Owner: remoteUserTag})
	defer st.Close()
	st.AddModelUser(state.EnvModelSpec{
		User:        admin.UserTag(),
		CreatedBy:   remoteUserTag,
		DisplayName: "Foo Bar"})

	s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "no-access", Owner: remoteUserTag}).Close()

	response, err := s.controller.AllModels()
	c.Assert(err, jc.ErrorIsNil)
	// The results are sorted.
	expected := []string{"dummymodel", "no-access", "owned", "user"}
	var obtained []string
	for _, env := range response.UserModels {
		obtained = append(obtained, env.Name)
		stateEnv, err := s.State.GetEnvironment(names.NewModelTag(env.UUID))
		c.Assert(err, jc.ErrorIsNil)
		s.checkEnvironmentMatches(c, env.Model, stateEnv)
	}
	c.Assert(obtained, jc.DeepEquals, expected)
}

func (s *controllerSuite) TestListBlockedModels(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "test"})
	defer st.Close()

	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")
	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	st.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	list, err := s.controller.ListBlockedModels()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(list.Models, jc.DeepEquals, []params.ModelBlockInfo{
		params.ModelBlockInfo{
			Name:     "dummymodel",
			UUID:     s.State.ModelUUID(),
			OwnerTag: s.AdminUserTag(c).String(),
			Blocks: []string{
				"BlockDestroy",
				"BlockChange",
			},
		},
		params.ModelBlockInfo{
			Name:     "test",
			UUID:     st.ModelUUID(),
			OwnerTag: s.AdminUserTag(c).String(),
			Blocks: []string{
				"BlockDestroy",
				"BlockChange",
			},
		},
	})

}

func (s *controllerSuite) TestListBlockedModelsNoBlocks(c *gc.C) {
	list, err := s.controller.ListBlockedModels()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(list.Models, gc.HasLen, 0)
}

func (s *controllerSuite) TestEnvironmentConfig(c *gc.C) {
	env, err := s.controller.EnvironmentConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Config["name"], gc.Equals, "dummymodel")
}

func (s *controllerSuite) TestEnvironmentConfigFromNonStateServer(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "test"})
	defer st.Close()

	authorizer := &apiservertesting.FakeAuthorizer{Tag: s.AdminUserTag(c)}
	controller, err := controller.NewControllerAPI(st, common.NewResources(), authorizer)
	c.Assert(err, jc.ErrorIsNil)
	env, err := controller.EnvironmentConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Config["name"], gc.Equals, "dummymodel")
}

func (s *controllerSuite) TestRemoveBlocks(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "test"})
	defer st.Close()

	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")
	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	st.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.controller.RemoveBlocks(params.RemoveBlocksArgs{All: true})
	c.Assert(err, jc.ErrorIsNil)

	blocks, err := s.State.AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 0)
}

func (s *controllerSuite) TestRemoveBlocksNotAll(c *gc.C) {
	err := s.controller.RemoveBlocks(params.RemoveBlocksArgs{})
	c.Assert(err, gc.ErrorMatches, "not supported")
}

func (s *controllerSuite) TestWatchAllEnvs(c *gc.C) {
	watcherId, err := s.controller.WatchAllEnvs()
	c.Assert(err, jc.ErrorIsNil)

	watcherAPI_, err := apiserver.NewAllWatcher(s.State, s.resources, s.authorizer, watcherId.AllWatcherId)
	c.Assert(err, jc.ErrorIsNil)
	watcherAPI := watcherAPI_.(*apiserver.SrvAllWatcher)
	defer func() {
		err := watcherAPI.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}()

	resultC := make(chan params.AllWatcherNextResults)
	go func() {
		result, err := watcherAPI.Next()
		c.Assert(err, jc.ErrorIsNil)
		resultC <- result
	}()

	select {
	case result := <-resultC:
		// Expect to see the initial environment be reported.
		deltas := result.Deltas
		c.Assert(deltas, gc.HasLen, 1)
		envInfo := deltas[0].Entity.(*multiwatcher.ModelInfo)
		c.Assert(envInfo.ModelUUID, gc.Equals, s.State.ModelUUID())
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
}

func (s *controllerSuite) TestModelStatus(c *gc.C) {
	otherEnvOwner := s.Factory.MakeEnvUser(c, nil)
	otherSt := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name:    "dummytoo",
		Owner:   otherEnvOwner.UserTag(),
		Prepare: true,
		ConfigAttrs: testing.Attrs{
			"state-server": false,
		},
	})
	defer otherSt.Close()

	s.Factory.MakeMachine(c, &factory.MachineParams{Jobs: []state.MachineJob{state.JobManageEnviron}})
	s.Factory.MakeMachine(c, &factory.MachineParams{Jobs: []state.MachineJob{state.JobHostUnits}})
	s.Factory.MakeService(c, &factory.ServiceParams{
		Charm: s.Factory.MakeCharm(c, nil),
	})

	otherFactory := factory.NewFactory(otherSt)
	otherFactory.MakeMachine(c, nil)
	otherFactory.MakeMachine(c, nil)
	otherFactory.MakeService(c, &factory.ServiceParams{
		Charm: otherFactory.MakeCharm(c, nil),
	})

	controllerEnvTag := s.State.ModelTag().String()
	hostedEnvTag := otherSt.ModelTag().String()

	req := params.Entities{
		Entities: []params.Entity{{Tag: controllerEnvTag}, {Tag: hostedEnvTag}},
	}
	results, err := s.controller.ModelStatus(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.DeepEquals, []params.ModelStatus{{
		ModelTag:           controllerEnvTag,
		HostedMachineCount: 1,
		ServiceCount:       1,
		OwnerTag:           "user-dummy-admin@local",
		Life:               params.Alive,
	}, {
		ModelTag:           hostedEnvTag,
		HostedMachineCount: 2,
		ServiceCount:       1,
		OwnerTag:           otherEnvOwner.UserTag().String(),
		Life:               params.Alive,
	}})
}
