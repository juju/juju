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

	"github.com/juju/juju/api"
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
}

var _ = gc.Suite(&destroyEnvironmentSuite{})

func (s *destroyEnvironmentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })
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

type testMetricSender struct {
	jtesting.Stub
}

func (t *testMetricSender) SendMetrics(st *state.State) error {
	t.AddCall("SendMetrics")
	return nil
}

func (s *destroyEnvironmentSuite) TestMetrics(c *gc.C) {
	metricSender := &testMetricSender{}
	s.PatchValue(common.SendMetrics, metricSender.SendMetrics)

	err := common.DestroyEnvironment(s.State, s.State.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	metricSender.CheckCalls(c, []jtesting.StubCall{{FuncName: "SendMetrics"}})
}

func (s *destroyEnvironmentSuite) TestDestroyEnvironmentManual(c *gc.C) {
	_, nonManager := s.setUpManual(c)

	// If there are any non-manager manual machines in state, DestroyEnvironment will
	// error. It will not set the Dying flag on the environment.
	err := common.DestroyEnvironment(s.State, s.State.EnvironTag())
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
	err = common.DestroyEnvironment(s.State, s.State.EnvironTag())
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

	err = common.DestroyEnvironment(s.State, s.State.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	runAllCleanups(c, s.State)

	// After DestroyEnvironment returns and all cleanup jobs have run, we should have:
	//   - all non-manager machines dying
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

func (s *destroyEnvironmentSuite) TestBlockDestroyDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyDestroyEnvironment")
	err := common.DestroyEnvironment(s.State, s.State.EnvironTag())
	s.AssertBlocked(c, err, "TestBlockDestroyDestroyEnvironment")
}

func (s *destroyEnvironmentSuite) TestBlockDestroyDestroyHostedEnvironment(c *gc.C) {
	otherSt := s.Factory.MakeEnvironment(c, nil)
	defer otherSt.Close()
	info := s.APIInfo(c)
	info.EnvironTag = otherSt.EnvironTag()
	apiState, err := api.Open(info, api.DefaultDialOpts())

	block := commontesting.NewBlockHelper(apiState)
	defer block.Close()

	block.BlockDestroyEnvironment(c, "TestBlockDestroyDestroyEnvironment")
	err = common.DestroyEnvironmentIncludingHosted(s.State, s.State.EnvironTag())
	s.AssertBlocked(c, err, "TestBlockDestroyDestroyEnvironment")
}

func (s *destroyEnvironmentSuite) TestBlockRemoveDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(c)
	s.BlockRemoveObject(c, "TestBlockRemoveDestroyEnvironment")
	err := common.DestroyEnvironment(s.State, s.State.EnvironTag())
	s.AssertBlocked(c, err, "TestBlockRemoveDestroyEnvironment")
}

func (s *destroyEnvironmentSuite) TestBlockChangesDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(c)
	// lock environment: can't destroy locked environment
	s.BlockAllChanges(c, "TestBlockChangesDestroyEnvironment")
	err := common.DestroyEnvironment(s.State, s.State.EnvironTag())
	s.AssertBlocked(c, err, "TestBlockChangesDestroyEnvironment")
}

type destroyTwoEnvironmentsSuite struct {
	testing.JujuConnSuite
	otherState     *state.State
	otherEnvOwner  names.UserTag
	otherEnvClient *client.Client
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

}

func (s *destroyTwoEnvironmentsSuite) TestCleanupEnvironResources(c *gc.C) {
	otherFactory := factory.NewFactory(s.otherState)
	m := otherFactory.MakeMachine(c, nil)
	otherFactory.MakeMachineNested(c, m.Id(), nil)

	err := common.DestroyEnvironment(s.otherState, s.otherState.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	// Assert that the machines are not removed until the cleanup runs.
	c.Assert(m.Refresh(), jc.ErrorIsNil)
	assertMachineCount(c, s.otherState, 2)
	runAllCleanups(c, s.otherState)
	assertAllMachinesDeadAndRemove(c, s.otherState)

	otherEnv, err := s.otherState.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(otherEnv.Life(), gc.Equals, state.Dying)

	c.Assert(s.otherState.ProcessDyingEnviron(), jc.ErrorIsNil)
	c.Assert(otherEnv.Refresh(), jc.ErrorIsNil)
	c.Assert(otherEnv.Life(), gc.Equals, state.Dead)

}

// The provisioner will remove dead machines once their backing instances are
// stopped. For the tests, we remove them directly.
func assertAllMachinesDeadAndRemove(c *gc.C, st *state.State) {
	machines, err := st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	for _, m := range machines {
		if m.IsManager() {
			continue
		}
		if _, isContainer := m.ParentId(); isContainer {
			continue
		}
		manual, err := m.IsManual()
		c.Assert(err, jc.ErrorIsNil)
		if manual {
			continue
		}

		c.Assert(m.Life(), gc.Equals, state.Dead)
		c.Assert(m.Remove(), jc.ErrorIsNil)
	}
}

func (s *destroyTwoEnvironmentsSuite) TestDifferentStateEnv(c *gc.C) {
	otherFactory := factory.NewFactory(s.otherState)
	otherFactory.MakeMachine(c, nil)
	m := otherFactory.MakeMachine(c, nil)
	otherFactory.MakeMachineNested(c, m.Id(), nil)

	// NOTE: pass in the main test State instance, which is 'bound'
	// to the state server environment.
	err := common.DestroyEnvironment(s.State, s.otherState.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	runAllCleanups(c, s.otherState)
	assertAllMachinesDeadAndRemove(c, s.otherState)

	otherEnv, err := s.otherState.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.otherState.ProcessDyingEnviron(), jc.ErrorIsNil)
	c.Assert(otherEnv.Refresh(), jc.ErrorIsNil)
	c.Assert(otherEnv.Life(), gc.Equals, state.Dead)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Alive)
}

func (s *destroyTwoEnvironmentsSuite) TestDestroyStateServerAfterNonStateServerIsDestroyed(c *gc.C) {
	otherFactory := factory.NewFactory(s.otherState)
	otherFactory.MakeMachine(c, nil)
	m := otherFactory.MakeMachine(c, nil)
	otherFactory.MakeMachineNested(c, m.Id(), nil)

	err := common.DestroyEnvironment(s.State, s.State.EnvironTag())
	c.Assert(err, gc.ErrorMatches, "failed to destroy environment: hosting 1 other environments")

	needsCleanup, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needsCleanup, jc.IsFalse)

	err = common.DestroyEnvironment(s.State, s.otherState.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	err = common.DestroyEnvironment(s.State, s.State.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	// Make sure we can continue to take the hosted environ down while the
	// controller environ is dying.
	runAllCleanups(c, s.otherState)
	assertAllMachinesDeadAndRemove(c, s.otherState)
	c.Assert(s.otherState.ProcessDyingEnviron(), jc.ErrorIsNil)

	otherEnv, err := s.otherState.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(otherEnv.Life(), gc.Equals, state.Dead)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
	c.Assert(s.State.ProcessDyingEnviron(), jc.ErrorIsNil)
	c.Assert(env.Refresh(), jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dead)
}

func (s *destroyTwoEnvironmentsSuite) TestDestroyStateServerAndNonStateServer(c *gc.C) {
	otherFactory := factory.NewFactory(s.otherState)
	otherFactory.MakeMachine(c, nil)
	m := otherFactory.MakeMachine(c, nil)
	otherFactory.MakeMachineNested(c, m.Id(), nil)

	err := common.DestroyEnvironmentIncludingHosted(s.State, s.State.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	runAllCleanups(c, s.State)
	runAllCleanups(c, s.otherState)
	assertAllMachinesDeadAndRemove(c, s.otherState)

	// Make sure we can continue to take the hosted environ down while the
	// controller environ is dying.
	c.Assert(s.otherState.ProcessDyingEnviron(), jc.ErrorIsNil)
}

func (s *destroyTwoEnvironmentsSuite) TestCanDestroyNonBlockedEnv(c *gc.C) {
	bh := commontesting.NewBlockHelper(s.APIState)
	defer bh.Close()

	bh.BlockDestroyEnvironment(c, "TestBlockDestroyDestroyEnvironment")

	err := common.DestroyEnvironment(s.State, s.otherState.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	err = common.DestroyEnvironment(s.State, s.State.EnvironTag())
	bh.AssertBlocked(c, err, "TestBlockDestroyDestroyEnvironment")
}

func runAllCleanups(c *gc.C, st *state.State) {
	needCleanup, err := st.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)

	for needCleanup {
		err := st.Cleanup()
		c.Assert(err, jc.ErrorIsNil)
		needCleanup, err = st.NeedsCleanup()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func assertMachineCount(c *gc.C, st *state.State, count int) {
	otherMachines, err := st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(otherMachines, gc.HasLen, count)
}
