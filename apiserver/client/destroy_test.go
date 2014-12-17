// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"

	"github.com/juju/juju/environs/config"
	"github.com/juju/utils"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type destroyEnvironmentSuite struct {
	baseSuite
}

type destroyTwoEnvironmentsSuite struct {
	baseSuite
	otherState     *state.State
	otherEnvOwner  names.UserTag
	otherEnvClient *client.Client
}

var _ = gc.Suite(&destroyEnvironmentSuite{})
var _ = gc.Suite(&destroyTwoEnvironmentsSuite{})

func (s *destroyTwoEnvironmentsSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	// Make the other environment
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfgAttrs := dummy.SampleConfig().Merge(coretesting.Attrs{"state-server": false, "agent-version": "1.2.3", "uuid": uuid.String()})
	delete(cfgAttrs, "admin-secret")
	cfg, err := config.New(config.NoDefaults, cfgAttrs)
	c.Assert(err, jc.ErrorIsNil)
	dummyProvider, err := environs.Provider("dummy")
	c.Assert(err, jc.ErrorIsNil)
	env, err := dummyProvider.Prepare(envtesting.BootstrapContext(c), cfg)
	c.Assert(err, jc.ErrorIsNil)

	s.otherEnvOwner = names.NewUserTag("jess@dummy")
	_, s.otherState, err = s.State.NewEnvironment(env.Config(), s.otherEnvOwner)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) {
		if s.otherState != nil {
			s.otherState.Close()
		}
		if s.State != nil {
			s.State.Close()
		}
		dummy.Reset()
	})

	// get the client for the other environment
	auth := apiservertesting.FakeAuthorizer{
		Tag:            s.otherEnvOwner,
		EnvironManager: false,
	}
	s.otherEnvClient, err = client.NewClient(s.otherState, common.NewResources(), auth)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *destroyTwoEnvironmentsSuite) TearDownTest(c *gc.C) {
	s.CleanupSuite.TearDownTest(c)
	s.baseSuite.TearDownTest(c)
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
func setUpInstances(c *gc.C, st *state.State, env environs.Environ) (m0, m1, m2 *state.Machine) {
	m0, err := st.AddMachine("precise", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	inst, _ := testing.AssertStartInstance(c, env, m0.Id())
	err = m0.SetProvisioned(inst.Id(), "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	m1, err = st.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	inst, _ = testing.AssertStartInstance(c, env, m1.Id())
	err = m1.SetProvisioned(inst.Id(), "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	m2, err = st.AddMachineInsideMachine(state.MachineTemplate{
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
	err := s.APIState.Client().DestroyEnvironment()
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("manually provisioned machines must first be destroyed with `juju destroy-machine %s`", nonManager.Id()))
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Alive)

	// If we remove the non-manager machine, it should pass.
	// Manager machines will remain.
	err = nonManager.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = nonManager.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().DestroyEnvironment()
	c.Assert(err, jc.ErrorIsNil)
	err = env.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyEnvironmentSuite) TestDestroyEnvironment(c *gc.C) {
	manager, nonManager, _ := setUpInstances(c, s.State, s.Environ)
	managerId, _ := manager.InstanceId()
	nonManagerId, _ := nonManager.InstanceId()

	instances, err := s.Environ.Instances([]instance.Id{managerId, nonManagerId})
	c.Assert(err, jc.ErrorIsNil)
	for _, inst := range instances {
		c.Assert(inst, gc.NotNil)
	}

	services, err := s.State.AllServices()
	c.Assert(err, jc.ErrorIsNil)

	err = s.APIState.Client().DestroyEnvironment()
	c.Assert(err, jc.ErrorIsNil)

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

	_, nonManager, _ := setUpInstances(c, s.State, s.Environ)
	nonManagerId, _ := nonManager.InstanceId()

	err := s.APIState.Client().DestroyEnvironment()
	c.Assert(err, jc.ErrorIsNil)
	for op := range ops {
		if op, ok := op.(dummy.OpStopInstances); ok {
			c.Assert(op.Ids, jc.SameContents, []instance.Id{nonManagerId})
			break
		}
	}
}

func (s *destroyTwoEnvironmentsSuite) TestCleanupEnvironDocs(c *gc.C) {
	// add instances to non-state-server environment
	manager, nonManager, _ := setUpInstances(c, s.otherState, s.Environ)
	managerId, _ := manager.InstanceId()
	nonManagerId, _ := nonManager.InstanceId()

	instances, err := s.Environ.Instances([]instance.Id{managerId, nonManagerId})
	c.Assert(err, jc.ErrorIsNil)
	for _, inst := range instances {
		c.Assert(inst, gc.NotNil)
	}

	err = s.otherEnvClient.DestroyEnvironment()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.otherState.Environment()
	c.Assert(errors.IsNotFound(err), jc.IsTrue)

	_, err = s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)

	// Check the logs that all environment specific documents were removed from state
	logOutput := c.GetTestLog()
	for kind, count := range map[string]int{
		"cleanups":          1,
		"constraints":       4,
		"settings":          1,
		"instanceData":      3,
		"requestednetworks": 3,
		"statuses":          3,
		"machines":          3,
	} {
		c.Assert(logOutput, jc.Contains, fmt.Sprintf("removed %d %s documents", count, kind))
	}
	c.Assert(logOutput, jc.Contains, "removed environment document")
}

func (s *destroyEnvironmentSuite) TestBlockDestroyDestroyEnvironment(c *gc.C) {
	// Setup environment
	setUpInstances(c, s.State, s.Environ)
	// lock environment: can't destroy locked environment
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"block-destroy-environment": true}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().DestroyEnvironment()
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)
}

func (s *destroyEnvironmentSuite) TestBlockRemoveDestroyEnvironment(c *gc.C) {
	// Setup environment
	setUpInstances(c, s.State, s.Environ)
	// lock environment: can't destroy locked environment
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"block-remove-object": true}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().DestroyEnvironment()
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)
}

func (s *destroyEnvironmentSuite) TestBlockChangesDestroyEnvironment(c *gc.C) {
	// Setup environment
	setUpInstances(c, s.State, s.Environ)
	// lock environment: can't destroy locked environment
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"block-all-changes": true}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().DestroyEnvironment()
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)
}
