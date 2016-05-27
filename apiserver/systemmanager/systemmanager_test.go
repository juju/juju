// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemmanager_test

import (
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/systemmanager"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type systemManagerSuite struct {
	jujutesting.JujuConnSuite

	systemManager *systemmanager.SystemManagerAPI
	resources     *common.Resources
	authorizer    apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&systemManagerSuite{})

func (s *systemManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}

	systemManager, err := systemmanager.NewSystemManagerAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.systemManager = systemManager

	loggo.GetLogger("juju.apiserver.systemmanager").SetLogLevel(loggo.TRACE)
}

func (s *systemManagerSuite) TestNewAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("mysql/0"),
	}
	endPoint, err := systemmanager.NewSystemManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *systemManagerSuite) TestNewAPIRefusesNonAdmins(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoEnvUser: true})
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: user.Tag(),
	}
	endPoint, err := systemmanager.NewSystemManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *systemManagerSuite) checkEnvironmentMatches(c *gc.C, env params.Environment, expected *state.Environment) {
	c.Check(env.Name, gc.Equals, expected.Name())
	c.Check(env.UUID, gc.Equals, expected.UUID())
	c.Check(env.OwnerTag, gc.Equals, expected.Owner().String())
}

func (s *systemManagerSuite) TestAllEnvironments(c *gc.C) {
	admin := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar"})

	s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "owned", Owner: admin.UserTag()}).Close()
	remoteUserTag := names.NewUserTag("user@remote")
	st := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "user", Owner: remoteUserTag})
	defer st.Close()
	st.AddEnvironmentUser(admin.UserTag(), remoteUserTag, "Foo Bar")

	s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "no-access", Owner: remoteUserTag}).Close()

	response, err := s.systemManager.AllEnvironments()
	c.Assert(err, jc.ErrorIsNil)
	// The results are sorted.
	expected := []string{"dummyenv", "no-access", "owned", "user"}
	var obtained []string
	for _, env := range response.UserEnvironments {
		obtained = append(obtained, env.Name)
		stateEnv, err := s.State.GetEnvironment(names.NewEnvironTag(env.UUID))
		c.Assert(err, jc.ErrorIsNil)
		s.checkEnvironmentMatches(c, env.Environment, stateEnv)
	}
	c.Assert(obtained, jc.DeepEquals, expected)
}

func (s *systemManagerSuite) TestListBlockedEnvironments(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "test"})
	defer st.Close()

	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")
	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	st.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	list, err := s.systemManager.ListBlockedEnvironments()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(list.Environments, jc.DeepEquals, []params.EnvironmentBlockInfo{
		params.EnvironmentBlockInfo{
			Name:     "dummyenv",
			UUID:     s.State.EnvironUUID(),
			OwnerTag: s.AdminUserTag(c).String(),
			Blocks: []string{
				"BlockDestroy",
				"BlockChange",
			},
		},
		params.EnvironmentBlockInfo{
			Name:     "test",
			UUID:     st.EnvironUUID(),
			OwnerTag: s.AdminUserTag(c).String(),
			Blocks: []string{
				"BlockDestroy",
				"BlockChange",
			},
		},
	})

}

func (s *systemManagerSuite) TestListBlockedEnvironmentsNoBlocks(c *gc.C) {
	list, err := s.systemManager.ListBlockedEnvironments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(list.Environments, gc.HasLen, 0)
}

func (s *systemManagerSuite) TestEnvironmentConfig(c *gc.C) {
	env, err := s.systemManager.EnvironmentConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Config["name"], gc.Equals, "dummyenv")
}

func (s *systemManagerSuite) TestEnvironmentConfigFromNonStateServer(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "test"})
	defer st.Close()

	authorizer := &apiservertesting.FakeAuthorizer{Tag: s.AdminUserTag(c)}
	systemManager, err := systemmanager.NewSystemManagerAPI(st, common.NewResources(), authorizer)
	c.Assert(err, jc.ErrorIsNil)
	env, err := systemManager.EnvironmentConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Config["name"], gc.Equals, "dummyenv")
}

func (s *systemManagerSuite) TestRemoveBlocks(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "test"})
	defer st.Close()

	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")
	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	st.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.systemManager.RemoveBlocks(params.RemoveBlocksArgs{All: true})
	c.Assert(err, jc.ErrorIsNil)

	blocks, err := s.State.AllBlocksForSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 0)
}

func (s *systemManagerSuite) TestRemoveBlocksNotAll(c *gc.C) {
	err := s.systemManager.RemoveBlocks(params.RemoveBlocksArgs{})
	c.Assert(err, gc.ErrorMatches, "not supported")
}

func (s *systemManagerSuite) TestWatchAllEnvs(c *gc.C) {
	watcherId, err := s.systemManager.WatchAllEnvs()
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
		envInfo := deltas[0].Entity.(*multiwatcher.EnvironmentInfo)
		c.Assert(envInfo.EnvUUID, gc.Equals, s.State.EnvironUUID())
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
}
