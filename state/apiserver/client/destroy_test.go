// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	coreerrors "launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	jc "launchpad.net/juju-core/testing/checkers"
)

type destroyEnvironmentSuite struct {
	baseSuite
}

var _ = gc.Suite(&destroyEnvironmentSuite{})

// setUpManual adds "manually provisioned" machines to state:
// one manager machine, and one non-manager.
func (s *destroyEnvironmentSuite) setUpManual(c *gc.C) (m0, m1 *state.Machine) {
	m0, err := s.State.AddMachine("precise", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	err = m0.SetProvisioned(instance.Id("manual:0"), "manual:0:fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	m1, err = s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = m1.SetProvisioned(instance.Id("manual:1"), "manual:1:fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	return m0, m1
}

// setUpInstances adds machines to state backed by instances:
// one manager machine, and one non-manager.
func (s *destroyEnvironmentSuite) setUpInstances(c *gc.C) (m0, m1 *state.Machine) {
	m0, err := s.State.AddMachine("precise", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	inst, _ := testing.AssertStartInstance(c, s.APIConn.Environ, m0.Id())
	err = m0.SetProvisioned(inst.Id(), "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	m1, err = s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	inst, _ = testing.AssertStartInstance(c, s.APIConn.Environ, m1.Id())
	err = m1.SetProvisioned(inst.Id(), "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	return m0, m1
}

func (s *destroyEnvironmentSuite) TestDestroyEnvironmentManual(c *gc.C) {
	_, nonManager := s.setUpManual(c)

	// If there are any non-manager manual machines in state, DestroyEnvironment will
	// error. It will not set the Dying flag on the environment.
	err := s.APIState.Client().DestroyEnvironment()
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("manually provisioned machines must first be destroyed with `juju destroy-machine %s`", nonManager.Id()))
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	c.Assert(env.Life(), gc.Equals, state.Alive)

	// If we remove the non-manager machine, it should pass.
	// Manager machines will remain.
	err = nonManager.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = nonManager.Remove()
	c.Assert(err, gc.IsNil)
	err = s.APIState.Client().DestroyEnvironment()
	c.Assert(err, gc.IsNil)
	err = env.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyEnvironmentSuite) TestDestroyEnvironment(c *gc.C) {
	manager, nonManager := s.setUpInstances(c)
	managerId, _ := manager.InstanceId()
	nonManagerId, _ := nonManager.InstanceId()

	instances, err := s.APIConn.Environ.Instances([]instance.Id{managerId, nonManagerId})
	c.Assert(err, gc.IsNil)
	for _, inst := range instances {
		c.Assert(inst, gc.NotNil)
	}

	services, err := s.State.AllServices()
	c.Assert(err, gc.IsNil)

	err = s.APIState.Client().DestroyEnvironment()
	c.Assert(err, gc.IsNil)

	// After DestroyEnvironment returns, we should have:
	//   - all non-manager instances stopped
	instances, err = s.APIConn.Environ.Instances([]instance.Id{managerId, nonManagerId})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(instances[0], gc.NotNil)
	c.Assert(instances[1], gc.IsNil)
	//   - all services in state are Dying or Dead (or removed altogether),
	//     after running the state Cleanups.
	needsCleanup, err := s.State.NeedsCleanup()
	c.Assert(err, gc.IsNil)
	c.Assert(needsCleanup, jc.IsTrue)
	err = s.State.Cleanup()
	c.Assert(err, gc.IsNil)
	for _, s := range services {
		err = s.Refresh()
		if err != nil {
			c.Assert(err, jc.Satisfies, coreerrors.IsNotFoundError)
		} else {
			c.Assert(s.Life(), gc.Not(gc.Equals), state.Alive)
		}
	}
	//   - environment is Dying
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}
