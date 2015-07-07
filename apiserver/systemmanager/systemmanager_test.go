// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemmanager_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/environmentmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/systemmanager"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type sysManagerBaseSuite struct {
	testing.JujuConnSuite

	sysmanager *systemmanager.SystemManagerAPI
	resources  *common.Resources
	authoriser apiservertesting.FakeAuthorizer
}

func (s *sysManagerBaseSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}

	loggo.GetLogger("juju.apiserver.systemmanager").SetLogLevel(loggo.TRACE)
}

func (s *sysManagerBaseSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authoriser.Tag = user
	sysmanager, err := systemmanager.NewSystemManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
	s.sysmanager = sysmanager
}

type sysManagerSuite struct {
	sysManagerBaseSuite
}

var _ = gc.Suite(&sysManagerSuite{})

func (s *sysManagerSuite) TestNewAPIAcceptsClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUserTag("external@remote")
	endPoint, err := systemmanager.NewSystemManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *sysManagerSuite) TestNewAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUnitTag("mysql/0")
	endPoint, err := systemmanager.NewSystemManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *sysManagerSuite) TestEnvironmentGet(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	env, err := s.sysmanager.EnvironmentGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Config["name"], gc.Equals, "dummyenv")
}

func (s *sysManagerSuite) TestEnvironmentGetNonAdminUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", NoEnvUser: false})

	s.setAPIUser(c, user.UserTag())
	env, err := s.sysmanager.EnvironmentGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Config["name"], gc.Equals, "dummyenv")
}

func (s *sysManagerSuite) TestEnvironmentGetFromNonStateServer(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "test"})
	defer st.Close()

	authorizer := &apiservertesting.FakeAuthorizer{Tag: s.AdminUserTag(c)}
	sysManager, err := systemmanager.NewSystemManagerAPI(st, common.NewResources(), authorizer)
	c.Assert(err, jc.ErrorIsNil)
	env, err := sysManager.EnvironmentGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Config["name"], gc.Equals, "dummyenv")
}

func (s *sysManagerSuite) TestUnauthorizedEnvironmentGet(c *gc.C) {
	owner := names.NewUserTag("external@remote")
	s.setAPIUser(c, owner)
	_, err := s.sysmanager.EnvironmentGet()
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *sysManagerSuite) TestListBlockedEnvironments(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "test"})
	defer st.Close()

	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")
	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	st.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	s.setAPIUser(c, s.AdminUserTag(c))
	list, err := s.sysmanager.ListBlockedEnvironments()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(list.Environments, jc.DeepEquals, []params.EnvironmentBlockInfo{
		params.EnvironmentBlockInfo{
			params.Environment{
				Name:     "dummyenv",
				UUID:     s.State.EnvironUUID(),
				OwnerTag: s.AdminUserTag(c).String(),
			},
			[]string{
				"BlockDestroy",
				"BlockChange",
			},
		},
		params.EnvironmentBlockInfo{
			params.Environment{
				Name:     "test",
				UUID:     st.EnvironUUID(),
				OwnerTag: s.AdminUserTag(c).String(),
			},
			[]string{
				"BlockDestroy",
				"BlockChange",
			},
		},
	})

}

func (s *sysManagerSuite) TestUnauthorizedListBlockedEnvironments(c *gc.C) {
	owner := names.NewUserTag("external@remote")
	s.setAPIUser(c, owner)
	_, err := s.sysmanager.ListBlockedEnvironments()
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *sysManagerSuite) TestListBlockedEnvironmentsNoBlocks(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	list, err := s.sysmanager.ListBlockedEnvironments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(list.Environments), gc.Equals, 0)
}

type destroyTwoEnvironmentsSuite struct {
	sysManagerBaseSuite
	commontesting.BlockHelper
	otherState    *state.State
	otherEnvOwner names.UserTag
	otherEnvUUID  string
}

var _ = gc.Suite(&destroyTwoEnvironmentsSuite{})

func (s *destroyTwoEnvironmentsSuite) SetUpTest(c *gc.C) {
	s.sysManagerBaseSuite.SetUpTest(c)

	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })

	s.otherEnvOwner = names.NewUserTag("jess@dummy")
	s.otherState = factory.NewFactory(s.State).MakeEnvironment(c, &factory.EnvParams{
		Name:    "dummytoo",
		Owner:   s.otherEnvOwner,
		Prepare: true,
		ConfigAttrs: jujutesting.Attrs{
			"state-server": false,
		},
	})
	s.AddCleanup(func(c *gc.C) { s.otherState.Close() })
	s.otherEnvUUID = s.otherState.EnvironUUID()
}

func (s *destroyTwoEnvironmentsSuite) destroyEnvironment(c *gc.C, envUUID string) error {
	envManager, err := environmentmanager.NewEnvironmentManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
	return envManager.DestroyEnvironment(params.DestroyEnvironmentArgs{names.NewEnvironTag(envUUID).String()})
}

func (s *destroyTwoEnvironmentsSuite) destroySystem(c *gc.C, envUUID string, destroyEnvs bool, ignoreBlocks bool) error {
	sysManager, err := systemmanager.NewSystemManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
	return sysManager.DestroySystem(params.DestroySystemArgs{
		EnvTag:       names.NewEnvironTag(envUUID).String(),
		DestroyEnvs:  destroyEnvs,
		IgnoreBlocks: ignoreBlocks,
	})
}

func (s *destroyTwoEnvironmentsSuite) TestDestroySystemNotASystem(c *gc.C) {
	err := s.destroySystem(c, s.otherState.EnvironUUID(), false, false)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("%q is not a system", s.otherState.EnvironUUID()))
}

func (s *destroyTwoEnvironmentsSuite) TestDestroySystemNotASystemFromHostedEnv(c *gc.C) {
	authoriser := apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}

	sysmanager, err := systemmanager.NewSystemManagerAPI(s.otherState, s.resources, authoriser)
	c.Assert(err, jc.ErrorIsNil)

	err = sysmanager.DestroySystem(params.DestroySystemArgs{
		EnvTag:       names.NewEnvironTag(s.otherState.EnvironUUID()).String(),
		DestroyEnvs:  true,
		IgnoreBlocks: true,
	})

	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("%q is not a system", s.otherState.EnvironUUID()))
}

func (s *destroyTwoEnvironmentsSuite) TestDestroySystemUnauthorized(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name:      "unautheduser",
		NoEnvUser: true,
	})

	authoriser := apiservertesting.FakeAuthorizer{
		Tag: user.UserTag(),
	}

	sysmanager, err := systemmanager.NewSystemManagerAPI(s.State, s.resources, authoriser)
	c.Assert(err, jc.ErrorIsNil)

	err = sysmanager.DestroySystem(params.DestroySystemArgs{
		EnvTag:       names.NewEnvironTag(s.State.EnvironUUID()).String(),
		DestroyEnvs:  true,
		IgnoreBlocks: true,
	})

	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *destroyTwoEnvironmentsSuite) TestDestroySystemKillsHostedEnvsWithBlocks(c *gc.C) {
	// Tests destroyEnvs=true, ignoreBlocks=true, hosted environments=Y, blocks=Y
	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	s.otherState.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.otherState.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")
	err := s.destroySystem(c, s.State.EnvironUUID(), true, true)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.otherState.Environment()
	c.Assert(errors.IsNotFound(err), jc.IsTrue)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyTwoEnvironmentsSuite) TestDestroySystemReturnsBlockedEnvironmentsErr(c *gc.C) {
	// Tests destroyEnvs=true, ignoreBlocks=false, hosted environments=Y, blocks=Y
	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	s.otherState.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.otherState.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")
	err := s.destroySystem(c, s.State.EnvironUUID(), true, false)
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)

	numBlocks, err := s.State.AllBlocksForSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 4)

	_, err = s.otherState.Environment()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *destroyTwoEnvironmentsSuite) TestDestroySystemKillsHostedEnvs(c *gc.C) {
	// Tests destroyEnvs=true, ignoreBlocks=false, hosted environments=Y, blocks=N
	err := s.destroySystem(c, s.State.EnvironUUID(), true, false)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.otherState.Environment()
	c.Assert(errors.IsNotFound(err), jc.IsTrue)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyTwoEnvironmentsSuite) TestDestroySystemLeavesBlocksIfNotKillAll(c *gc.C) {
	// Tests destroyEnvs=false, ignoreBlocks=true, hosted environments=Y, blocks=Y
	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	s.otherState.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.otherState.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.destroySystem(c, s.State.EnvironUUID(), false, true)
	c.Assert(err, gc.ErrorMatches, "state server environment cannot be destroyed before all other environments are destroyed")

	numBlocks, err := s.State.AllBlocksForSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 4)
}

func (s *destroyTwoEnvironmentsSuite) TestDestroySystemNoHostedEnvs(c *gc.C) {
	// Tests destroyEnvs=false, ignoreBlocks=false, hosted environments=N, blocks=N
	err := s.destroyEnvironment(c, s.otherEnvUUID)
	c.Assert(err, jc.ErrorIsNil)

	err = s.destroySystem(c, s.State.EnvironUUID(), false, false)
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyTwoEnvironmentsSuite) TestDestroySystemNoHostedEnvsWithBlock(c *gc.C) {
	// Tests destroyEnvs=false, ignoreBlocks=true, hosted environments=N, blocks=Y
	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	err := s.destroyEnvironment(c, s.otherEnvUUID)
	c.Assert(err, jc.ErrorIsNil)

	err = s.destroySystem(c, s.State.EnvironUUID(), false, true)
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyTwoEnvironmentsSuite) TestDestroySystemNoHostedEnvsWithBlockFail(c *gc.C) {
	// Tests destroyEnvs=false, ignoreBlocks=false, hosted environments=N, blocks=Y
	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	err := s.destroyEnvironment(c, s.otherEnvUUID)
	c.Assert(err, jc.ErrorIsNil)

	err = s.destroySystem(c, s.State.EnvironUUID(), false, false)
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)

	numBlocks, err := s.State.AllBlocksForSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 2)
}
