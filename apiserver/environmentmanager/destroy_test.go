// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environmentmanager_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/environmentmanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type destroyEnvironmentSuite struct {
	envManagerBaseSuite
	commontesting.BlockHelper
}

func (s *destroyEnvironmentSuite) SetUpTest(c *gc.C) {
	s.envManagerBaseSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })
}

var _ = gc.Suite(&destroyEnvironmentSuite{})

// setUpManual adds "manually provisioned" machines to state:
// one manager machine, and one non-manager.
func (s *destroyEnvironmentSuite) setUpManual(c *gc.C) (m0, m1 *state.Machine) {
	m0, err := s.State.AddMachine("precise", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	err = m0.SetProvisioned(instance.Id("manual:0"), "manual:0:fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	m1, err = s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = m1.SetProvisioned(instance.Id("manual:1"), "manual:1:fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	return m0, m1
}

// setUpInstances adds machines to state backed by instances:
// one manager machine, one non-manager, and a container in the
// non-manager.
func (s *destroyEnvironmentSuite) setUpInstances(c *gc.C) (m0, m1, m2 *state.Machine) {
	m0, err := s.State.AddMachine("precise", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	inst, _ := testing.AssertStartInstance(c, s.Environ, m0.Id())
	err = m0.SetProvisioned(inst.Id(), "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	m1, err = s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	inst, _ = testing.AssertStartInstance(c, s.Environ, m1.Id())
	err = m1.SetProvisioned(inst.Id(), "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	m2, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "precise",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, m1.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	err = m2.SetProvisioned("container0", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	return m0, m1, m2
}

func (s *destroyEnvironmentSuite) destroyEnvironment(c *gc.C, envUUID string) error {
	envManager, err := environmentmanager.NewEnvironmentManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
	return envManager.DestroyEnvironment(params.DestroyEnvironmentArgs{envUUID})
}

func (s *destroyEnvironmentSuite) TestDestroyEnvironmentManual(c *gc.C) {
	_, nonManager := s.setUpManual(c)

	// If there are any non-manager manual machines in state, DestroyEnvironment will
	// error. It will not set the Dying flag on the environment.
	err := s.destroyEnvironment(c, s.State.EnvironUUID())
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("failed to destroy environment: manually provisioned machines must first be destroyed with `juju destroy-machine %s`", nonManager.Id()))
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Alive)

	// If we remove the non-manager machine, it should pass.
	// Manager machines will remain.
	err = nonManager.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = nonManager.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.destroyEnvironment(c, s.State.EnvironUUID())
	c.Assert(err, jc.ErrorIsNil)
	err = env.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyEnvironmentSuite) TestDestroyEnvironment(c *gc.C) {
	manager, nonManager, _ := s.setUpInstances(c)
	managerId, _ := manager.InstanceId()
	nonManagerId, _ := nonManager.InstanceId()

	instances, err := s.Environ.Instances([]instance.Id{managerId, nonManagerId})
	c.Assert(err, jc.ErrorIsNil)
	for _, inst := range instances {
		c.Assert(inst, gc.NotNil)
	}

	services, err := s.State.AllServices()
	c.Assert(err, jc.ErrorIsNil)

	err = s.destroyEnvironment(c, s.State.EnvironUUID())
	c.Assert(err, jc.ErrorIsNil)

	// After DestroyEnvironment returns, check that the following 3 things
	// have happened:
	//   1 - all non-manager instances stopped
	instances, err = s.Environ.Instances([]instance.Id{managerId, nonManagerId})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(instances[0], gc.NotNil)
	c.Assert(instances[1], jc.ErrorIsNil)
	//   2 - all services in state are Dying or Dead (or removed altogether),
	//       after running the state Cleanups.
	needsCleanup, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needsCleanup, jc.IsTrue)
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	for _, s := range services {
		err = s.Refresh()
		if err != nil {
			c.Assert(err, jc.Satisfies, errors.IsNotFound)
		} else {
			c.Assert(s.Life(), gc.Not(gc.Equals), state.Alive)
		}
	}
	//   3 - environment is Dying
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyEnvironmentSuite) TestDestroyEnvironmentWithContainers(c *gc.C) {
	ops := make(chan dummy.Operation, 500)
	dummy.Listen(ops)

	_, nonManager, _ := s.setUpInstances(c)
	nonManagerId, _ := nonManager.InstanceId()

	err := s.destroyEnvironment(c, s.State.EnvironUUID())
	c.Assert(err, jc.ErrorIsNil)
	for op := range ops {
		if op, ok := op.(dummy.OpStopInstances); ok {
			c.Assert(op.Ids, jc.SameContents, []instance.Id{nonManagerId})
			break
		}
	}
}

func (s *destroyEnvironmentSuite) TestBlockDestroyDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyDestroyEnvironment")
	err := s.destroyEnvironment(c, s.State.EnvironUUID())
	s.AssertBlocked(c, err, "TestBlockDestroyDestroyEnvironment")
}

func (s *destroyEnvironmentSuite) TestBlockRemoveDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(c)
	s.BlockRemoveObject(c, "TestBlockRemoveDestroyEnvironment")
	err := s.destroyEnvironment(c, s.State.EnvironUUID())
	s.AssertBlocked(c, err, "TestBlockRemoveDestroyEnvironment")
}

func (s *destroyEnvironmentSuite) TestBlockChangesDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(c)
	// lock environment: can't destroy locked environment
	s.BlockAllChanges(c, "TestBlockChangesDestroyEnvironment")
	err := s.destroyEnvironment(c, s.State.EnvironUUID())
	s.AssertBlocked(c, err, "TestBlockChangesDestroyEnvironment")
}

func (s *destroyEnvironmentSuite) TestCannotDestroyUnsharedEnvironment(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("non-admin@remote"))
	err := s.destroyEnvironment(c, s.State.EnvironUUID())
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

type destroyTwoEnvironmentsSuite struct {
	envManagerBaseSuite
	commontesting.BlockHelper
	otherState    *state.State
	otherEnvOwner names.UserTag
	otherEnvUUID  string
}

var _ = gc.Suite(&destroyTwoEnvironmentsSuite{})

func (s *destroyTwoEnvironmentsSuite) SetUpTest(c *gc.C) {
	s.envManagerBaseSuite.SetUpTest(c)

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
	return envManager.DestroyEnvironment(params.DestroyEnvironmentArgs{envUUID})
}

func (s *destroyTwoEnvironmentsSuite) destroySystem(c *gc.C, envUUID string, killAll bool, ignoreBlocks bool) error {
	envManager, err := environmentmanager.NewEnvironmentManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
	return envManager.DestroySystem(params.DestroySystemArgs{
		EnvUUID:      envUUID,
		KillEnvs:     killAll,
		IgnoreBlocks: ignoreBlocks,
	})
}

func (s *destroyTwoEnvironmentsSuite) TestCleanupEnvironDocs(c *gc.C) {
	otherFactory := factory.NewFactory(s.otherState)
	otherFactory.MakeMachine(c, nil)
	m := otherFactory.MakeMachine(c, nil)
	otherFactory.MakeMachineNested(c, m.Id(), nil)

	err := s.destroyEnvironment(c, s.otherEnvUUID)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.otherState.Environment()
	c.Assert(errors.IsNotFound(err), jc.IsTrue)

	_, err = s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.otherState.EnsureEnvironmentRemoved(), jc.ErrorIsNil)
}

func (s *destroyTwoEnvironmentsSuite) TestDestroyStateServerAfterNonStateServerIsDestroyed(c *gc.C) {
	err := s.destroyEnvironment(c, s.State.EnvironUUID())
	c.Assert(err, gc.ErrorMatches, "failed to destroy environment: state server environment cannot be destroyed before all other environments are destroyed")
	err = s.destroyEnvironment(c, s.otherEnvUUID)
	c.Assert(err, jc.ErrorIsNil)
	err = s.destroyEnvironment(c, s.State.EnvironUUID())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *destroyTwoEnvironmentsSuite) TestCanDestroyNonBlockedEnv(c *gc.C) {
	s.BlockDestroyEnvironment(c, "TestBlockDestroyDestroyEnvironment")
	err := s.destroyEnvironment(c, s.otherEnvUUID)
	c.Assert(err, jc.ErrorIsNil)
	err = s.destroyEnvironment(c, s.State.EnvironUUID())
	s.AssertBlocked(c, err, "TestBlockDestroyDestroyEnvironment")
}

func (s *destroyTwoEnvironmentsSuite) TestDestroySystemNotASystem(c *gc.C) {
	err := s.destroySystem(c, s.otherState.EnvironUUID(), false, false)
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

	envmanager, err := environmentmanager.NewEnvironmentManagerAPI(s.State, s.resources, authoriser)
	c.Assert(err, jc.ErrorIsNil)

	err = envmanager.DestroySystem(params.DestroySystemArgs{
		EnvUUID:      s.State.EnvironUUID(),
		KillEnvs:     true,
		IgnoreBlocks: true,
	})

	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *destroyTwoEnvironmentsSuite) TestDestroySystemKillsHostedEnvsWithBlocks(c *gc.C) {
	// Tests killEnvs=true, ignoreBlocks=true, hosted environments=Y, blocks=Y
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
	// Tests killEnvs=true, ignoreBlocks=false, hosted environments=Y, blocks=Y
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
	// Tests killEnvs=true, ignoreBlocks=false, hosted environments=Y, blocks=N
	err := s.destroySystem(c, s.State.EnvironUUID(), true, false)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.otherState.Environment()
	c.Assert(errors.IsNotFound(err), jc.IsTrue)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyTwoEnvironmentsSuite) TestDestroySystemLeavesBlocksIfNotKillAll(c *gc.C) {
	// Tests killEnvs=false, ignoreBlocks=true, hosted environments=Y, blocks=Y
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
	// Tests killEnvs=false, ignoreBlocks=false, hosted environments=N, blocks=N
	err := s.destroyEnvironment(c, s.otherState.EnvironUUID())
	c.Assert(err, jc.ErrorIsNil)

	err = s.destroySystem(c, s.State.EnvironUUID(), false, false)
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyTwoEnvironmentsSuite) TestDestroySystemNoHostedEnvsWithBlock(c *gc.C) {
	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	err := s.destroyEnvironment(c, s.otherState.EnvironUUID())
	c.Assert(err, jc.ErrorIsNil)

	err = s.destroySystem(c, s.State.EnvironUUID(), false, true)
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyTwoEnvironmentsSuite) TestDestroySystemNoHostedEnvsWithBlockFail(c *gc.C) {
	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	err := s.destroyEnvironment(c, s.otherState.EnvironUUID())
	c.Assert(err, jc.ErrorIsNil)

	err = s.destroySystem(c, s.State.EnvironUUID(), false, false)
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)

	numBlocks, err := s.State.AllBlocksForSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 2)
}
