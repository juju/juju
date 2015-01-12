// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type destroyEnvironmentSuite struct {
	baseSuite
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
func (s *destroyEnvironmentSuite) setUpInstances(st *state.State, c *gc.C) (m0, m1, m2 *state.Machine) {
	m0, err := st.AddMachine("precise", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	inst, _ := testing.AssertStartInstance(c, s.Environ, m0.Id())
	err = m0.SetProvisioned(inst.Id(), "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	m1, err = st.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	inst, _ = testing.AssertStartInstance(c, s.Environ, m1.Id())
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
	manager, nonManager, _ := s.setUpInstances(s.State, c)
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

func (s *destroyEnvironmentSuite) TestDestroyOtherEnvironment(c *gc.C) {
	cfg := dummy.SampleConfig().Merge(coretesting.Attrs{"state-server": false, "state-id": "2", "agent-version": "1.13.2"})
	delete(cfg, "uuid")

	envUser := names.NewUserTag("jess@dummy")
	st2 := s.Factory.MakeEnvironment(c, &factory.EnvParams{Owner: envUser, ConfigAttrs: cfg}) //coretesting.Attrs{"type": "dummy", "state-server": false}
	defer st2.Close()

	// try to destroy state server environment
	err := s.APIState.Client().DestroyEnvironment()
	c.Assert(err, gc.ErrorMatches, "state server environment cannot be destroyed before all other environments are destroyed")

	// get the client for the other environ
	auth := apiservertesting.FakeAuthorizer{
		Tag:            envUser,
		EnvironManager: false,
	}
	client2, err := client.NewClient(st2, common.NewResources(), auth)
	c.Assert(err, jc.ErrorIsNil)

	// destroy the other environment
	err = client2.DestroyEnvironment()
	c.Assert(err, jc.ErrorIsNil)

	// now we can destroy the state server environment
	err = s.APIState.Client().DestroyEnvironment()
	c.Assert(err, jc.ErrorIsNil)
}

// func (s *destroyEnvironmentSuite) cleanupEnvironDocs(){
// 		// add instances to non-state-server environment
// 	manager, nonManager, _ := s.setUpInstances(st2, c)
// 	managerId, _ := manager.InstanceId()
// 	nonManagerId, _ := nonManager.InstanceId()

// 	instances, err := s.Environ.Instances([]instance.Id{managerId, nonManagerId})
// 	c.Assert(err, jc.ErrorIsNil)
// 	for _, inst := range instances {
// 		c.Assert(inst, gc.NotNil)
// 	}

// 	services, err := st2.AllServices()
// 	c.Assert(err, jc.ErrorIsNil)
// }

func (s *destroyEnvironmentSuite) TestDestroyEnvironmentWithContainers(c *gc.C) {
	ops := make(chan dummy.Operation, 500)
	dummy.Listen(ops)

	_, nonManager, _ := s.setUpInstances(s.State, c)
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

func (s *destroyEnvironmentSuite) TestBlockDestroyDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(s.State, c)
	// lock environment: can't destroy locked environment
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"block-destroy-environment": true}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().DestroyEnvironment()
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)
}

func (s *destroyEnvironmentSuite) TestBlockRemoveDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(s.State, c)
	// lock environment: can't destroy locked environment
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"block-remove-object": true}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().DestroyEnvironment()
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)
}

func (s *destroyEnvironmentSuite) TestBlockChangesDestroyEnvironment(c *gc.C) {
	// Setup environment
	s.setUpInstances(s.State, c)
	// lock environment: can't destroy locked environment
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"block-all-changes": true}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().DestroyEnvironment()
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)
}
