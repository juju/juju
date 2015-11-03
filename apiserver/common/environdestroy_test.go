// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type destroyEnvironmentSuite struct {
	testing.JujuConnSuite
	commontesting.BlockHelper
	metricSender *testMetricSender
}

var _ = gc.Suite(&destroyEnvironmentSuite{})

func (s *destroyEnvironmentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })

	s.metricSender = &testMetricSender{}
	s.PatchValue(common.SendMetrics, s.metricSender.SendMetrics)
}

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

func (s *destroyEnvironmentSuite) TestDestroyEnvironmentManual(c *gc.C) {
	_, nonManager := s.setUpManual(c)

	// If there are any non-manager manual machines in state, DestroyEnvironment will
	// error. It will not set the Dying flag on the environment.
	err := common.DestroyEnvironment(s.State, s.State.EnvironTag(), false)
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
	err = common.DestroyEnvironment(s.State, s.State.EnvironTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	err = env.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)

	s.metricSender.CheckCalls(c, []jtesting.StubCall{{FuncName: "SendMetrics"}})
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

	err = common.DestroyEnvironment(s.State, s.State.EnvironTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	needsCleanup, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needsCleanup, jc.IsTrue)
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)

	// After DestroyEnvironment returns, we should have:
	//   - all non-manager machines dead
	assertLife(c, manager, state.Alive)
	// Note: we leave the machine in a dead state and rely on the provisioner
	// to stop the backing instances, remove the dead machines and finally
	// remove all environment docs from state.
	assertLife(c, nonManager, state.Dead)

	//   - all services in state are Dying or Dead (or removed altogether),
	//     after running the state Cleanups.
	for _, s := range services {
		err = s.Refresh()
		if err != nil {
			c.Assert(err, jc.Satisfies, errors.IsNotFound)
		} else {
			c.Assert(s.Life(), gc.Not(gc.Equals), state.Alive)
		}
	}
	//   - environment is Dying or Dead.
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Not(gc.Equals), state.Alive)
}

func assertLife(c *gc.C, entity state.Living, life state.Life) {
	err := entity.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Life(), gc.Equals, life)
}

func (s *destroyEnvironmentSuite) TestDestroyEnvironmentWithContainers(c *gc.C) {
	manager, nonManager, container := s.setUpInstances(c)

	err := common.DestroyEnvironment(s.State, s.State.EnvironTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	assertCleanupCount(c, s.State, 2)

	err = container.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	c.Assert(nonManager.Refresh(), jc.ErrorIsNil)
	c.Assert(nonManager.Life(), gc.Equals, state.Dead)

	c.Assert(manager.Refresh(), jc.ErrorIsNil)
	c.Assert(manager.Life(), gc.Equals, state.Alive)
}

func (s *destroyEnvironmentSuite) TestBlockDestroyDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyDestroyEnvironment")
	err := common.DestroyEnvironment(s.State, s.State.EnvironTag(), false)
	s.AssertBlocked(c, err, "TestBlockDestroyDestroyEnvironment")
}

func (s *destroyEnvironmentSuite) TestBlockRemoveDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(c)
	s.BlockRemoveObject(c, "TestBlockRemoveDestroyEnvironment")
	err := common.DestroyEnvironment(s.State, s.State.EnvironTag(), false)
	s.AssertBlocked(c, err, "TestBlockRemoveDestroyEnvironment")
}

func (s *destroyEnvironmentSuite) TestBlockChangesDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(c)
	// lock environment: can't destroy locked environment
	s.BlockAllChanges(c, "TestBlockChangesDestroyEnvironment")
	err := common.DestroyEnvironment(s.State, s.State.EnvironTag(), false)
	s.AssertBlocked(c, err, "TestBlockChangesDestroyEnvironment")
	s.metricSender.CheckCalls(c, []jtesting.StubCall{})
}

type destroyTwoEnvironmentsSuite struct {
	testing.JujuConnSuite
	otherState     *state.State
	otherEnvOwner  names.UserTag
	otherEnvClient *client.Client
	metricSender   *testMetricSender
}

var _ = gc.Suite(&destroyTwoEnvironmentsSuite{})

func (s *destroyTwoEnvironmentsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	_, err := s.State.AddUser("jess", "jess", "", "test")
	c.Assert(err, jc.ErrorIsNil)
	s.otherEnvOwner = names.NewUserTag("jess")
	s.otherState = factory.NewFactory(s.State).MakeEnvironment(c, &factory.EnvParams{
		Owner:   s.otherEnvOwner,
		Prepare: true,
		ConfigAttrs: jujutesting.Attrs{
			"state-server": false,
		},
	})
	s.AddCleanup(func(*gc.C) { s.otherState.Close() })

	// get the client for the other environment
	auth := apiservertesting.FakeAuthorizer{
		Tag:            s.otherEnvOwner,
		EnvironManager: false,
	}
	s.otherEnvClient, err = client.NewClient(s.otherState, common.NewResources(), auth)
	c.Assert(err, jc.ErrorIsNil)

	s.metricSender = &testMetricSender{}
	s.PatchValue(common.SendMetrics, s.metricSender.SendMetrics)
}

func (s *destroyTwoEnvironmentsSuite) TestDestroyEnvironmentWithContainer(c *gc.C) {
	otherFactory := factory.NewFactory(s.otherState)
	m1 := otherFactory.MakeMachine(c, nil)
	container := otherFactory.MakeMachineNested(c, m1.Id(), nil)

	err := common.DestroyEnvironment(s.otherState, s.otherState.EnvironTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	processDyingEnviron(c, s.otherState)

	err = container.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *destroyTwoEnvironmentsSuite) TestWaitsForResourceRemoval(c *gc.C) {
	otherFactory := factory.NewFactory(s.otherState)
	otherFactory.MakeMachine(c, nil)
	m := otherFactory.MakeMachine(c, nil)
	otherFactory.MakeMachineNested(c, m.Id(), nil)

	err := common.DestroyEnvironment(s.otherState, s.otherState.EnvironTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	err = s.otherState.ProcessDyingEnviron()
	c.Assert(err, gc.ErrorMatches, `environment not empty, found 3 machine\(s\)`)

	env, err := s.otherState.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)

	_, err = s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	s.metricSender.CheckCalls(c, []jtesting.StubCall{{FuncName: "SendMetrics"}})
}

func (s *destroyTwoEnvironmentsSuite) TestDifferentStateEnv(c *gc.C) {
	otherFactory := factory.NewFactory(s.otherState)
	otherFactory.MakeMachine(c, nil)
	m := otherFactory.MakeMachine(c, nil)
	otherFactory.MakeMachineNested(c, m.Id(), nil)

	// NOTE: pass in the main test State instance, which is 'bound'
	// to the state server environment.
	err := common.DestroyEnvironment(s.State, s.otherState.EnvironTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	processDyingEnviron(c, s.otherState)

	env, err := s.otherState.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dead)

	_, err = s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	s.metricSender.CheckCalls(c, []jtesting.StubCall{{FuncName: "SendMetrics"}})
}

func (s *destroyTwoEnvironmentsSuite) TestDestroyStateServerAfterNonStateServerIsDestroyed(c *gc.C) {
	err := common.DestroyEnvironment(s.State, s.State.EnvironTag(), false)
	c.Assert(err, gc.ErrorMatches, "failed to destroy environment: hosting 1 other environments")
	err = common.DestroyEnvironment(s.State, s.otherState.EnvironTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	err = common.DestroyEnvironment(s.State, s.State.EnvironTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	s.metricSender.CheckCalls(c, []jtesting.StubCall{{FuncName: "SendMetrics"}, {FuncName: "SendMetrics"}})
}

func (s *destroyTwoEnvironmentsSuite) TestDestroyStateServerAndNonStateServer(c *gc.C) {
	err := common.DestroyEnvironment(s.State, s.State.EnvironTag(), true)
	c.Assert(err, jc.ErrorIsNil)
	s.metricSender.CheckCalls(c, []jtesting.StubCall{{FuncName: "SendMetrics"}})
}

func (s *destroyTwoEnvironmentsSuite) TestCanDestroyNonBlockedEnv(c *gc.C) {
	bh := commontesting.NewBlockHelper(s.APIState)
	defer bh.Close()

	bh.BlockDestroyEnvironment(c, "TestBlockDestroyDestroyEnvironment")

	err := common.DestroyEnvironment(s.State, s.otherState.EnvironTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	err = common.DestroyEnvironment(s.State, s.State.EnvironTag(), false)
	bh.AssertBlocked(c, err, "TestBlockDestroyDestroyEnvironment")

	s.metricSender.CheckCalls(c, []jtesting.StubCall{{FuncName: "SendMetrics"}})
}

type testMetricSender struct {
	jtesting.Stub
}

func (t *testMetricSender) SendMetrics(st *state.State) error {
	t.AddCall("SendMetrics")
	return nil
}

func processDyingEnviron(c *gc.C, st *state.State) {
	assertCleanupCount(c, st, 2)
	machines, err := st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	for _, machine := range machines {
		if machine.Life() == state.Dead {
			c.Assert(machine.Remove(), jc.ErrorIsNil)
		}
	}
	c.Assert(st.ProcessDyingEnviron(), jc.ErrorIsNil)
}

// assertCleanupCount is useful because certain cleanups cause other cleanups
// to be queued; it makes more sense to just run cleanup again than to unpick
// object destruction so that we run the cleanups inline while running cleanups.
func assertCleanupCount(c *gc.C, st *state.State, count int) {
	for i := 0; i < count; i++ {
		c.Logf("checking cleanups %d", i)
		assertNeedsCleanup(c, st)

		err := st.Cleanup()
		c.Assert(err, jc.ErrorIsNil)
	}
	assertDoesNotNeedCleanup(c, st)
}

func assertNeedsCleanup(c *gc.C, st *state.State) {
	actual, err := st.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, jc.IsTrue)
}

func assertDoesNotNeedCleanup(c *gc.C, st *state.State) {
	actual, err := st.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, jc.IsFalse)
}
