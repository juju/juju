// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type destroyEnvironmentSuite struct {
	baseSuite
}

type destroyTwoEnvironmentsSuite struct {
	jujutesting.BaseSuite
	gitjujutesting.MgoSuite
	State          *state.State
	owner          names.UserTag
	otherState     *state.State
	otherEnvOwner  names.UserTag
	otherEnvClient *client.Client
}

var _ = gc.Suite(&destroyEnvironmentSuite{})
var _ = gc.Suite(&destroyTwoEnvironmentsSuite{})

func (s *destroyTwoEnvironmentsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.owner = names.NewLocalUserTag("test-admin")
	st, err := state.Initialize(s.owner, TestingMongoInfo(), jujutesting.EnvironConfig(c), TestingDialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	s.State = st

	s.otherEnvOwner = names.NewUserTag("jess@dummy")
	s.otherState = factory.NewFactory(s.State).MakeEnvironment(c, &factory.EnvParams{
		Owner: s.otherEnvOwner,
		// ConfigAttrs: jujutesting.Attrs{"type": "dummy"},
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

// TestingMongoInfo returns information suitable for
// connecting to the testing state server's mongo database.
func TestingMongoInfo() *mongo.MongoInfo {
	return &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{gitjujutesting.MgoServer.Addr()},
			CACert: jujutesting.CACert,
		},
	}
}

// TestingDialOpts returns configuration parameters for
// connecting to the testing state server.
func TestingDialOpts() mongo.DialOpts {
	return mongo.DialOpts{
		Timeout: jujutesting.LongWait,
	}
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
	f := factory.NewFactory(st)
	m0 := f.MakeMachine(c, nil)
	inst, _ := testing.AssertStartInstance(c, env, m0.Id())
	err = m0.SetProvisioned(inst.Id(), "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	m1 := f.MakeMachine(c, nil)
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
	manager, nonManager, _ := setUpInstances(c, s.OtherState, s.Environ)
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
	otherFactory := factory.NewFactory(s.otherState)
	otherFactory.MakeMachine(c, nil)
	m := otherFactory.MakeMachine(c, nil)
	otherFactory.MakeMachineNested(c, m.Id(), nil)

	err := s.otherEnvClient.DestroyEnvironment()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.otherState.Environment()
	c.Assert(errors.IsNotFound(err), jc.IsTrue)

	_, err = s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.otherState.NoEnvDocs(), jc.ErrorIsNil)
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
