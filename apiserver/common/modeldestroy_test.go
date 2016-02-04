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

type destroyModelSuite struct {
	testing.JujuConnSuite
	commontesting.BlockHelper
}

var _ = gc.Suite(&destroyModelSuite{})

func (s *destroyModelSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })
}

// setUpManual adds "manually provisioned" machines to state:
// one manager machine, and one non-manager.
func (s *destroyModelSuite) setUpManual(c *gc.C) (m0, m1 *state.Machine) {
	m0, err := s.State.AddMachine("precise", state.JobManageModel)
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
func (s *destroyModelSuite) setUpInstances(c *gc.C) (m0, m1, m2 *state.Machine) {
	m0, err := s.State.AddMachine("precise", state.JobManageModel)
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

func (s *destroyModelSuite) TestMetrics(c *gc.C) {
	metricSender := &testMetricSender{}
	s.PatchValue(common.SendMetrics, metricSender.SendMetrics)

	err := common.DestroyModel(s.State, s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	metricSender.CheckCalls(c, []jtesting.StubCall{{FuncName: "SendMetrics"}})
}

func (s *destroyModelSuite) TestDestroyModelManual(c *gc.C) {
	_, nonManager := s.setUpManual(c)

	// If there are any non-manager manual machines in state, DestroyModel will
	// error. It will not set the Dying flag on the environment.
	err := common.DestroyModel(s.State, s.State.ModelTag())
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("failed to destroy model: manually provisioned machines must first be destroyed with `juju destroy-machine %s`", nonManager.Id()))
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Alive)

	// If we remove the non-manager machine, it should pass.
	// Manager machines will remain.
	err = nonManager.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = nonManager.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = common.DestroyModel(s.State, s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	err = env.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)

}

func (s *destroyModelSuite) TestDestroyModel(c *gc.C) {
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

	err = common.DestroyModel(s.State, s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	runAllCleanups(c, s.State)

	// After DestroyModel returns and all cleanup jobs have run, we should have:
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
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Not(gc.Equals), state.Alive)
}

func assertLife(c *gc.C, entity state.Living, life state.Life) {
	err := entity.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Life(), gc.Equals, life)
}

func (s *destroyModelSuite) TestBlockDestroyDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(c)
	s.BlockDestroyModel(c, "TestBlockDestroyDestroyModel")
	err := common.DestroyModel(s.State, s.State.ModelTag())
	s.AssertBlocked(c, err, "TestBlockDestroyDestroyModel")
}

func (s *destroyModelSuite) TestBlockDestroyDestroyHostedModel(c *gc.C) {
	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()
	info := s.APIInfo(c)
	info.ModelTag = otherSt.ModelTag()
	apiState, err := api.Open(info, api.DefaultDialOpts())

	block := commontesting.NewBlockHelper(apiState)
	defer block.Close()

	block.BlockDestroyModel(c, "TestBlockDestroyDestroyModel")
	err = common.DestroyModelIncludingHosted(s.State, s.State.ModelTag())
	s.AssertBlocked(c, err, "TestBlockDestroyDestroyModel")
}

func (s *destroyModelSuite) TestBlockRemoveDestroyModel(c *gc.C) {
	// Setup model
	s.setUpInstances(c)
	s.BlockRemoveObject(c, "TestBlockRemoveDestroyModel")
	err := common.DestroyModel(s.State, s.State.ModelTag())
	s.AssertBlocked(c, err, "TestBlockRemoveDestroyModel")
}

func (s *destroyModelSuite) TestBlockChangesDestroyModel(c *gc.C) {
	// Setup model
	s.setUpInstances(c)
	// lock model: can't destroy locked model
	s.BlockAllChanges(c, "TestBlockChangesDestroyModel")
	err := common.DestroyModel(s.State, s.State.ModelTag())
	s.AssertBlocked(c, err, "TestBlockChangesDestroyModel")
}

type destroyTwoModelsSuite struct {
	testing.JujuConnSuite
	otherState     *state.State
	otherEnvOwner  names.UserTag
	otherEnvClient *client.Client
}

var _ = gc.Suite(&destroyTwoModelsSuite{})

func (s *destroyTwoModelsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	_, err := s.State.AddUser("jess", "jess", "", "test")
	c.Assert(err, jc.ErrorIsNil)
	s.otherEnvOwner = names.NewUserTag("jess")
	s.otherState = factory.NewFactory(s.State).MakeModel(c, &factory.ModelParams{
		Owner:   s.otherEnvOwner,
		Prepare: true,
		ConfigAttrs: jujutesting.Attrs{
			"controller": false,
		},
	})
	s.AddCleanup(func(*gc.C) { s.otherState.Close() })

	// get the client for the other model
	auth := apiservertesting.FakeAuthorizer{
		Tag:            s.otherEnvOwner,
		EnvironManager: false,
	}
	s.otherEnvClient, err = client.NewClient(s.otherState, common.NewResources(), auth)
	c.Assert(err, jc.ErrorIsNil)

}

func (s *destroyTwoModelsSuite) TestCleanupModelResources(c *gc.C) {
	otherFactory := factory.NewFactory(s.otherState)
	m := otherFactory.MakeMachine(c, nil)
	otherFactory.MakeMachineNested(c, m.Id(), nil)

	err := common.DestroyModel(s.otherState, s.otherState.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	// Assert that the machines are not removed until the cleanup runs.
	c.Assert(m.Refresh(), jc.ErrorIsNil)
	assertMachineCount(c, s.otherState, 2)
	runAllCleanups(c, s.otherState)
	assertAllMachinesDeadAndRemove(c, s.otherState)

	otherEnv, err := s.otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(otherEnv.Life(), gc.Equals, state.Dying)

	c.Assert(s.otherState.ProcessDyingModel(), jc.ErrorIsNil)
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

func (s *destroyTwoModelsSuite) TestDifferentStateModel(c *gc.C) {
	otherFactory := factory.NewFactory(s.otherState)
	otherFactory.MakeMachine(c, nil)
	m := otherFactory.MakeMachine(c, nil)
	otherFactory.MakeMachineNested(c, m.Id(), nil)

	// NOTE: pass in the main test State instance, which is 'bound'
	// to the controller model.
	err := common.DestroyModel(s.State, s.otherState.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	runAllCleanups(c, s.otherState)
	assertAllMachinesDeadAndRemove(c, s.otherState)

	otherEnv, err := s.otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.otherState.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(otherEnv.Refresh(), jc.ErrorIsNil)
	c.Assert(otherEnv.Life(), gc.Equals, state.Dead)

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Alive)
}

func (s *destroyTwoModelsSuite) TestDestroyControllerAfterNonControllerIsDestroyed(c *gc.C) {
	otherFactory := factory.NewFactory(s.otherState)
	otherFactory.MakeMachine(c, nil)
	m := otherFactory.MakeMachine(c, nil)
	otherFactory.MakeMachineNested(c, m.Id(), nil)

	err := common.DestroyModel(s.State, s.State.ModelTag())
	c.Assert(err, gc.ErrorMatches, "failed to destroy model: hosting 1 other models")

	needsCleanup, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needsCleanup, jc.IsFalse)

	err = common.DestroyModel(s.State, s.otherState.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	err = common.DestroyModel(s.State, s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	// Make sure we can continue to take the hosted model down while the
	// controller environ is dying.
	runAllCleanups(c, s.otherState)
	assertAllMachinesDeadAndRemove(c, s.otherState)
	c.Assert(s.otherState.ProcessDyingModel(), jc.ErrorIsNil)

	otherEnv, err := s.otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(otherEnv.Life(), gc.Equals, state.Dead)

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
	c.Assert(s.State.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(env.Refresh(), jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dead)
}

func (s *destroyTwoModelsSuite) TestDestroyControllerAndNonController(c *gc.C) {
	otherFactory := factory.NewFactory(s.otherState)
	otherFactory.MakeMachine(c, nil)
	m := otherFactory.MakeMachine(c, nil)
	otherFactory.MakeMachineNested(c, m.Id(), nil)

	err := common.DestroyModelIncludingHosted(s.State, s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	runAllCleanups(c, s.State)
	runAllCleanups(c, s.otherState)
	assertAllMachinesDeadAndRemove(c, s.otherState)

	// Make sure we can continue to take the hosted model down while the
	// controller model is dying.
	c.Assert(s.otherState.ProcessDyingModel(), jc.ErrorIsNil)
}

func (s *destroyTwoModelsSuite) TestCanDestroyNonBlockedModel(c *gc.C) {
	bh := commontesting.NewBlockHelper(s.APIState)
	defer bh.Close()

	bh.BlockDestroyModel(c, "TestBlockDestroyDestroyModel")

	err := common.DestroyModel(s.State, s.otherState.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	err = common.DestroyModel(s.State, s.State.ModelTag())
	bh.AssertBlocked(c, err, "TestBlockDestroyDestroyModel")
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
