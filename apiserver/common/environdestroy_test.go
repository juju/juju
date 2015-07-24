// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jtesting "github.com/juju/testing"
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

	err = common.DestroyEnvironment(s.State, s.State.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	s.metricSender.CheckCalls(c, []jtesting.StubCall{{FuncName: "SendMetrics"}})

	// After DestroyEnvironment returns, we should have:
	//   - all non-manager instances stopped
	instances, err = s.Environ.Instances([]instance.Id{managerId, nonManagerId})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(instances[0], gc.NotNil)
	c.Assert(instances[1], jc.ErrorIsNil)
	//   - all services in state are Dying or Dead (or removed altogether),
	//     after running the state Cleanups.
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
	//   - environment is Dying
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyEnvironmentSuite) TestDestroyEnvironmentWithContainers(c *gc.C) {
	ops := make(chan dummy.Operation, 500)
	dummy.Listen(ops)

	_, nonManager, _ := s.setUpInstances(c)
	nonManagerId, _ := nonManager.InstanceId()

	err := common.DestroyEnvironment(s.State, s.State.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)
	for op := range ops {
		if op, ok := op.(dummy.OpStopInstances); ok {
			c.Assert(op.Ids, jc.SameContents, []instance.Id{nonManagerId})
			break
		}
	}

	s.metricSender.CheckCalls(c, []jtesting.StubCall{{FuncName: "SendMetrics"}})
}

func (s *destroyEnvironmentSuite) TestBlockDestroyDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyDestroyEnvironment")
	err := common.DestroyEnvironment(s.State, s.State.EnvironTag())
	s.AssertBlocked(c, err, "TestBlockDestroyDestroyEnvironment")
	s.metricSender.CheckCalls(c, []jtesting.StubCall{})
}

func (s *destroyEnvironmentSuite) TestBlockRemoveDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(c)
	s.BlockRemoveObject(c, "TestBlockRemoveDestroyEnvironment")
	err := common.DestroyEnvironment(s.State, s.State.EnvironTag())
	s.AssertBlocked(c, err, "TestBlockRemoveDestroyEnvironment")
	s.metricSender.CheckCalls(c, []jtesting.StubCall{})
}

func (s *destroyEnvironmentSuite) TestBlockChangesDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(c)
	// lock environment: can't destroy locked environment
	s.BlockAllChanges(c, "TestBlockChangesDestroyEnvironment")
	err := common.DestroyEnvironment(s.State, s.State.EnvironTag())
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

func (s *destroyTwoEnvironmentsSuite) TestCleanupEnvironDocs(c *gc.C) {
	otherFactory := factory.NewFactory(s.otherState)
	otherFactory.MakeMachine(c, nil)
	m := otherFactory.MakeMachine(c, nil)
	otherFactory.MakeMachineNested(c, m.Id(), nil)

	err := common.DestroyEnvironment(s.otherState, s.otherState.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.otherState.Environment()
	c.Assert(errors.IsNotFound(err), jc.IsTrue)

	_, err = s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.otherState.EnsureEnvironmentRemoved(), jc.ErrorIsNil)
	s.metricSender.CheckCalls(c, []jtesting.StubCall{{FuncName: "SendMetrics"}})
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

	_, err = s.otherState.Environment()
	c.Assert(errors.IsNotFound(err), jc.IsTrue)

	_, err = s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.otherState.EnsureEnvironmentRemoved(), jc.ErrorIsNil)

	s.metricSender.CheckCalls(c, []jtesting.StubCall{{FuncName: "SendMetrics"}})
}

func (s *destroyTwoEnvironmentsSuite) TestDestroyStateServerAfterNonStateServerIsDestroyed(c *gc.C) {
	err := common.DestroyEnvironment(s.State, s.State.EnvironTag())
	c.Assert(err, gc.ErrorMatches, "failed to destroy environment: hosting 1 other environments")
	err = common.DestroyEnvironment(s.State, s.otherState.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)
	err = common.DestroyEnvironment(s.State, s.State.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)
	s.metricSender.CheckCalls(c, []jtesting.StubCall{{FuncName: "SendMetrics"}, {FuncName: "SendMetrics"}})
}

func (s *destroyTwoEnvironmentsSuite) TestCanDestroyNonBlockedEnv(c *gc.C) {
	bh := commontesting.NewBlockHelper(s.APIState)
	defer bh.Close()

	bh.BlockDestroyEnvironment(c, "TestBlockDestroyDestroyEnvironment")

	err := common.DestroyEnvironment(s.State, s.otherState.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	err = common.DestroyEnvironment(s.State, s.State.EnvironTag())
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
