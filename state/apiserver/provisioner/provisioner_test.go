// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	commontesting "launchpad.net/juju-core/state/apiserver/common/testing"
	"launchpad.net/juju-core/state/apiserver/provisioner"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type provisionerSuite struct {
	testing.JujuConnSuite

	machines []*state.Machine

	authorizer  apiservertesting.FakeAuthorizer
	resources   *common.Resources
	provisioner *provisioner.ProvisionerAPI
}

var _ = gc.Suite(&provisionerSuite{})

func (s *provisionerSuite) SetUpTest(c *gc.C) {
	s.setUpTest(c, false)
}

func (s *provisionerSuite) setUpTest(c *gc.C, withStateServer bool) {
	s.JujuConnSuite.SetUpTest(c)

	// Reset previous machines (if any) and create 3 machines
	// for the tests, plus an optional state server machine.
	s.machines = nil
	// Note that the specific machine ids allocated are assumed
	// to be numerically consecutive from zero.
	if withStateServer {
		s.machines = append(s.machines, testing.AddStateServerMachine(c, s.State))
	}
	for i := 0; i < 3; i++ {
		machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Check(err, gc.IsNil)
		s.machines = append(s.machines, machine)
	}

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming we logged in as the environment manager.
	s.authorizer = apiservertesting.FakeAuthorizer{
		LoggedIn:       true,
		EnvironManager: true,
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()

	// Create a provisioner API for the machine.
	provisionerAPI, err := provisioner.NewProvisionerAPI(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, gc.IsNil)
	s.provisioner = provisionerAPI
}

type withoutStateServerSuite struct {
	provisionerSuite
	*commontesting.EnvironWatcherTest
}

var _ = gc.Suite(&withoutStateServerSuite{})

func (s *withoutStateServerSuite) SetUpTest(c *gc.C) {
	s.setUpTest(c, false)
	s.EnvironWatcherTest = commontesting.NewEnvironWatcherTest(s.provisioner, s.State, s.resources, commontesting.HasSecrets)
}

func (s *withoutStateServerSuite) TestProvisionerFailsWithNonMachineAgentNonManagerUser(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.MachineAgent = false
	anAuthorizer.EnvironManager = true
	// Works with an environment manager, which is not a machine agent.
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.IsNil)
	c.Assert(aProvisioner, gc.NotNil)

	// But fails with neither a machine agent or an environment manager.
	anAuthorizer.EnvironManager = false
	aProvisioner, err = provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.NotNil)
	c.Assert(aProvisioner, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *withoutStateServerSuite) TestSetPasswords(c *gc.C) {
	args := params.PasswordChanges{
		Changes: []params.PasswordChange{
			{Tag: s.machines[0].Tag(), Password: "xxx0-1234567890123457890"},
			{Tag: s.machines[1].Tag(), Password: "xxx1-1234567890123457890"},
			{Tag: s.machines[2].Tag(), Password: "xxx2-1234567890123457890"},
			{Tag: "machine-42", Password: "foo"},
			{Tag: "unit-foo-0", Password: "zzz"},
			{Tag: "service-bar", Password: "abc"},
		},
	}
	results, err := s.provisioner.SetPasswords(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes to both machines succeeded.
	for i, machine := range s.machines {
		c.Logf("trying %q password", machine.Tag())
		err = machine.Refresh()
		c.Assert(err, gc.IsNil)
		changed := machine.PasswordValid(fmt.Sprintf("xxx%d-1234567890123457890", i))
		c.Assert(changed, jc.IsTrue)
	}
}

func (s *withoutStateServerSuite) TestShortSetPasswords(c *gc.C) {
	args := params.PasswordChanges{
		Changes: []params.PasswordChange{
			{Tag: s.machines[1].Tag(), Password: "xxx1"},
		},
	}
	results, err := s.provisioner.SetPasswords(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"password is only 4 bytes long, and is not a valid Agent password")
}

func (s *withoutStateServerSuite) TestLifeAsMachineAgent(c *gc.C) {
	// NOTE: This and the next call serve to test the two
	// different authorization schemes:
	// 1. Machine agents can access their own machine and
	// any container that has their own machine as parent;
	// 2. Environment managers can access any machine without
	// a parent.
	// There's no need to repeat this test for each method,
	// because the authorization logic is common.

	// Login as a machine agent for machine 0.
	anAuthorizer := s.authorizer
	anAuthorizer.MachineAgent = true
	anAuthorizer.EnvironManager = false
	anAuthorizer.Tag = s.machines[0].Tag()
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.IsNil)
	c.Assert(aProvisioner, gc.NotNil)

	// Make the machine dead before trying to add containers.
	err = s.machines[0].EnsureDead()
	c.Assert(err, gc.IsNil)

	// Create some containers to work on.
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	var containers []*state.Machine
	for i := 0; i < 3; i++ {
		container, err := s.State.AddMachineInsideMachine(template, s.machines[0].Id(), instance.LXC)
		c.Check(err, gc.IsNil)
		containers = append(containers, container)
	}
	// Make one container dead.
	err = containers[1].EnsureDead()
	c.Assert(err, gc.IsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag()},
		{Tag: s.machines[1].Tag()},
		{Tag: containers[0].Tag()},
		{Tag: containers[1].Tag()},
		{Tag: containers[2].Tag()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "service-bar"},
	}}
	result, err := aProvisioner.Life(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "dead"},
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "alive"},
			{Life: "dead"},
			{Life: "alive"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutStateServerSuite) TestLifeAsEnvironManager(c *gc.C) {
	err := s.machines[1].EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.machines[1].Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.machines[0].Life(), gc.Equals, state.Alive)
	c.Assert(s.machines[1].Life(), gc.Equals, state.Dead)
	c.Assert(s.machines[2].Life(), gc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag()},
		{Tag: s.machines[1].Tag()},
		{Tag: s.machines[2].Tag()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "service-bar"},
	}}
	result, err := s.provisioner.Life(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "alive"},
			{Life: "dead"},
			{Life: "alive"},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Remove the subordinate and make sure it's detected.
	err = s.machines[1].Remove()
	c.Assert(err, gc.IsNil)
	err = s.machines[1].Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	result, err = s.provisioner.Life(params.Entities{
		Entities: []params.Entity{
			{Tag: s.machines[1].Tag()},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.NotFoundError("machine 1")},
		},
	})
}

func (s *withoutStateServerSuite) TestRemove(c *gc.C) {
	err := s.machines[1].EnsureDead()
	c.Assert(err, gc.IsNil)
	s.assertLife(c, 0, state.Alive)
	s.assertLife(c, 1, state.Dead)
	s.assertLife(c, 2, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag()},
		{Tag: s.machines[1].Tag()},
		{Tag: s.machines[2].Tag()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "service-bar"},
	}}
	result, err := s.provisioner.Remove(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: `cannot remove entity "machine-0": still alive`}},
			{nil},
			{&params.Error{Message: `cannot remove entity "machine-2": still alive`}},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertLife(c, 0, state.Alive)
	err = s.machines[1].Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	s.assertLife(c, 2, state.Alive)
}

func (s *withoutStateServerSuite) TestSetStatus(c *gc.C) {
	err := s.machines[0].SetStatus(params.StatusStarted, "blah", nil)
	c.Assert(err, gc.IsNil)
	err = s.machines[1].SetStatus(params.StatusStopped, "foo", nil)
	c.Assert(err, gc.IsNil)
	err = s.machines[2].SetStatus(params.StatusError, "not really", nil)
	c.Assert(err, gc.IsNil)

	args := params.SetStatus{
		Entities: []params.SetEntityStatus{
			{Tag: s.machines[0].Tag(), Status: params.StatusError, Info: "not really"},
			{Tag: s.machines[1].Tag(), Status: params.StatusStopped, Info: "foobar"},
			{Tag: s.machines[2].Tag(), Status: params.StatusStarted, Info: "again"},
			{Tag: "machine-42", Status: params.StatusStarted, Info: "blah"},
			{Tag: "unit-foo-0", Status: params.StatusStopped, Info: "foobar"},
			{Tag: "service-bar", Status: params.StatusStopped, Info: "foobar"},
		}}
	result, err := s.provisioner.SetStatus(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertStatus(c, 0, params.StatusError, "not really")
	s.assertStatus(c, 1, params.StatusStopped, "foobar")
	s.assertStatus(c, 2, params.StatusStarted, "again")
}

func (s *withoutStateServerSuite) TestEnsureDead(c *gc.C) {
	err := s.machines[1].EnsureDead()
	c.Assert(err, gc.IsNil)
	s.assertLife(c, 0, state.Alive)
	s.assertLife(c, 1, state.Dead)
	s.assertLife(c, 2, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag()},
		{Tag: s.machines[1].Tag()},
		{Tag: s.machines[2].Tag()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "service-bar"},
	}}
	result, err := s.provisioner.EnsureDead(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertLife(c, 0, state.Dead)
	s.assertLife(c, 1, state.Dead)
	s.assertLife(c, 2, state.Dead)
}

func (s *withoutStateServerSuite) assertLife(c *gc.C, index int, expectLife state.Life) {
	err := s.machines[index].Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.machines[index].Life(), gc.Equals, expectLife)
}

func (s *withoutStateServerSuite) assertStatus(c *gc.C, index int, expectStatus params.Status, expectInfo string) {
	status, info, _, err := s.machines[index].Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, expectStatus)
	c.Assert(info, gc.Equals, expectInfo)
}

func (s *withoutStateServerSuite) TestWatchContainers(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.WatchContainers{Params: []params.WatchContainer{
		{MachineTag: s.machines[0].Tag(), ContainerType: string(instance.LXC)},
		{MachineTag: s.machines[1].Tag(), ContainerType: string(instance.KVM)},
		{MachineTag: "machine-42", ContainerType: ""},
		{MachineTag: "unit-foo-0", ContainerType: ""},
		{MachineTag: "service-bar", ContainerType: ""},
	}}
	result, err := s.provisioner.WatchContainers(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{StringsWatcherId: "1", Changes: []string{}},
			{StringsWatcherId: "2", Changes: []string{}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 2)
	m0Watcher := s.resources.Get("1")
	defer statetesting.AssertStop(c, m0Watcher)
	m1Watcher := s.resources.Get("2")
	defer statetesting.AssertStop(c, m1Watcher)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc0 := statetesting.NewStringsWatcherC(c, s.State, m0Watcher.(state.StringsWatcher))
	wc0.AssertNoChange()
	wc1 := statetesting.NewStringsWatcherC(c, s.State, m1Watcher.(state.StringsWatcher))
	wc1.AssertNoChange()
}

func (s *withoutStateServerSuite) TestWatchAllContainers(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.WatchContainers{Params: []params.WatchContainer{
		{MachineTag: s.machines[0].Tag()},
		{MachineTag: s.machines[1].Tag()},
		{MachineTag: "machine-42"},
		{MachineTag: "unit-foo-0"},
		{MachineTag: "service-bar"},
	}}
	result, err := s.provisioner.WatchAllContainers(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{StringsWatcherId: "1", Changes: []string{}},
			{StringsWatcherId: "2", Changes: []string{}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 2)
	m0Watcher := s.resources.Get("1")
	defer statetesting.AssertStop(c, m0Watcher)
	m1Watcher := s.resources.Get("2")
	defer statetesting.AssertStop(c, m1Watcher)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc0 := statetesting.NewStringsWatcherC(c, s.State, m0Watcher.(state.StringsWatcher))
	wc0.AssertNoChange()
	wc1 := statetesting.NewStringsWatcherC(c, s.State, m1Watcher.(state.StringsWatcher))
	wc1.AssertNoChange()
}

func (s *withoutStateServerSuite) TestEnvironConfigNonManager(c *gc.C) {
	// Now test it with a non-environment manager and make sure
	// the secret attributes are masked.
	anAuthorizer := s.authorizer
	anAuthorizer.MachineAgent = true
	anAuthorizer.EnvironManager = false
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources,
		anAuthorizer)
	c.Assert(err, gc.IsNil)
	s.AssertEnvironConfig(c, aProvisioner, commontesting.NoSecrets)
}

func (s *withoutStateServerSuite) TestStatus(c *gc.C) {
	err := s.machines[0].SetStatus(params.StatusStarted, "blah", nil)
	c.Assert(err, gc.IsNil)
	err = s.machines[1].SetStatus(params.StatusStopped, "foo", nil)
	c.Assert(err, gc.IsNil)
	err = s.machines[2].SetStatus(params.StatusError, "not really", nil)
	c.Assert(err, gc.IsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag()},
		{Tag: s.machines[1].Tag()},
		{Tag: s.machines[2].Tag()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "service-bar"},
	}}
	result, err := s.provisioner.Status(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Status: params.StatusStarted, Info: "blah"},
			{Status: params.StatusStopped, Info: "foo"},
			{Status: params.StatusError, Info: "not really"},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutStateServerSuite) TestSeries(c *gc.C) {
	// Add a machine with different series.
	foobarMachine, err := s.State.AddMachine("foobar", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag()},
		{Tag: foobarMachine.Tag()},
		{Tag: s.machines[2].Tag()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "service-bar"},
	}}
	result, err := s.provisioner.Series(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: s.machines[0].Series()},
			{Result: foobarMachine.Series()},
			{Result: s.machines[2].Series()},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutStateServerSuite) TestConstraints(c *gc.C) {
	// Add a machine with some constraints.
	template := state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: constraints.MustParse("cpu-cores=123", "mem=8G"),
	}
	consMachine, err := s.State.AddOneMachine(template)
	c.Assert(err, gc.IsNil)

	machine0Constraints, err := s.machines[0].Constraints()
	c.Assert(err, gc.IsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag()},
		{Tag: consMachine.Tag()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "service-bar"},
	}}
	result, err := s.provisioner.Constraints(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ConstraintsResults{
		Results: []params.ConstraintsResult{
			{Constraints: machine0Constraints},
			{Constraints: template.Constraints},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutStateServerSuite) TestSetProvisioned(c *gc.C) {
	// Provision machine 0 first.
	hwChars := instance.MustParseHardware("arch=i386", "mem=4G")
	err := s.machines[0].SetProvisioned("i-am", "fake_nonce", &hwChars)
	c.Assert(err, gc.IsNil)

	args := params.SetProvisioned{Machines: []params.MachineSetProvisioned{
		{Tag: s.machines[0].Tag(), InstanceId: "i-was", Nonce: "fake_nonce", Characteristics: nil},
		{Tag: s.machines[1].Tag(), InstanceId: "i-will", Nonce: "fake_nonce", Characteristics: &hwChars},
		{Tag: s.machines[2].Tag(), InstanceId: "i-am-too", Nonce: "fake", Characteristics: nil},
		{Tag: "machine-42", InstanceId: "", Nonce: "", Characteristics: nil},
		{Tag: "unit-foo-0", InstanceId: "", Nonce: "", Characteristics: nil},
		{Tag: "service-bar", InstanceId: "", Nonce: "", Characteristics: nil},
	}}
	result, err := s.provisioner.SetProvisioned(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{
				Message: `cannot set instance data for machine "0": already set`,
			}},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify machine 1 and 2 were provisioned.
	c.Assert(s.machines[1].Refresh(), gc.IsNil)
	c.Assert(s.machines[2].Refresh(), gc.IsNil)

	instanceId, err := s.machines[1].InstanceId()
	c.Assert(err, gc.IsNil)
	c.Check(instanceId, gc.Equals, instance.Id("i-will"))
	instanceId, err = s.machines[2].InstanceId()
	c.Assert(err, gc.IsNil)
	c.Check(instanceId, gc.Equals, instance.Id("i-am-too"))
	c.Check(s.machines[1].CheckProvisioned("fake_nonce"), jc.IsTrue)
	c.Check(s.machines[2].CheckProvisioned("fake"), jc.IsTrue)
	gotHardware, err := s.machines[1].HardwareCharacteristics()
	c.Assert(err, gc.IsNil)
	c.Check(gotHardware, gc.DeepEquals, &hwChars)
}

func (s *withoutStateServerSuite) TestInstanceId(c *gc.C) {
	// Provision 2 machines first.
	err := s.machines[0].SetProvisioned("i-am", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	hwChars := instance.MustParseHardware("arch=i386", "mem=4G")
	err = s.machines[1].SetProvisioned("i-am-not", "fake_nonce", &hwChars)
	c.Assert(err, gc.IsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag()},
		{Tag: s.machines[1].Tag()},
		{Tag: s.machines[2].Tag()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "service-bar"},
	}}
	result, err := s.provisioner.InstanceId(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "i-am"},
			{Result: "i-am-not"},
			{Error: apiservertesting.NotProvisionedError("2")},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutStateServerSuite) TestWatchEnvironMachines(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	result, err := s.provisioner.WatchEnvironMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{"0", "1", "2"},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	// Make sure WatchEnvironMachines fails with a machine agent login.
	anAuthorizer := s.authorizer
	anAuthorizer.MachineAgent = true
	anAuthorizer.EnvironManager = false
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.IsNil)

	result, err = aProvisioner.WatchEnvironMachines()
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{})
}

func (s *withoutStateServerSuite) TestToolsNothing(c *gc.C) {
	// Not an error to watch nothing
	results, err := s.provisioner.Tools(params.Entities{})
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *withoutStateServerSuite) TestContainerConfig(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	newCfg, err := cfg.Apply(map[string]interface{}{
		"http-proxy": "http://proxy.example.com:9000",
	})
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(newCfg, cfg)
	c.Assert(err, gc.IsNil)
	expectedProxy := osenv.ProxySettings{
		Http: "http://proxy.example.com:9000",
	}

	results, err := s.provisioner.ContainerConfig()
	c.Check(err, gc.IsNil)
	c.Check(results.ProviderType, gc.Equals, "dummy")
	c.Check(results.AuthorizedKeys, gc.Equals, coretesting.FakeAuthKeys)
	c.Check(results.SSLHostnameVerification, jc.IsTrue)
	c.Check(results.Proxy, gc.DeepEquals, expectedProxy)
	c.Check(results.AptProxy, gc.DeepEquals, expectedProxy)
}

func (s *withoutStateServerSuite) TestToolsRefusesWrongAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = "machine-12354"
	anAuthorizer.EnvironManager = false
	anAuthorizer.MachineAgent = true
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, gc.IsNil)
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.machines[0].Tag()}},
	}
	results, err := aProvisioner.Tools(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 1)
	toolResult := results.Results[0]
	c.Assert(toolResult.Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *withoutStateServerSuite) TestToolsForAgent(c *gc.C) {
	cur := version.Current
	agent := params.Entity{Tag: s.machines[0].Tag()}

	// The machine must have its existing tools set before we query for the
	// next tools. This is so that we can grab Arch and Series without
	// having to pass it in again
	err := s.machines[0].SetAgentVersion(version.Current)
	c.Assert(err, gc.IsNil)

	args := params.Entities{Entities: []params.Entity{agent}}
	results, err := s.provisioner.Tools(args)
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	agentTools := results.Results[0].Tools
	c.Check(agentTools.URL, gc.Not(gc.Equals), "")
	c.Check(agentTools.Version, gc.DeepEquals, cur)
}

func (s *withoutStateServerSuite) TestSetSupportedContainers(c *gc.C) {
	args := params.MachineContainersParams{
		Params: []params.MachineContainers{
			{
				MachineTag:     "machine-0",
				ContainerTypes: []instance.ContainerType{instance.LXC},
			},
			{
				MachineTag:     "machine-1",
				ContainerTypes: []instance.ContainerType{instance.LXC, instance.KVM},
			},
		},
	}
	results, err := s.provisioner.SetSupportedContainers(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	for _, result := range results.Results {
		c.Assert(result.Error, gc.IsNil)
	}
	m0, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	containers, ok := m0.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, []instance.ContainerType{instance.LXC})
	m1, err := s.State.Machine("1")
	c.Assert(err, gc.IsNil)
	containers, ok = m1.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, []instance.ContainerType{instance.LXC, instance.KVM})
}

func (s *withoutStateServerSuite) TestSetSupportedContainersPermissions(c *gc.C) {
	// Login as a machine agent for machine 0.
	anAuthorizer := s.authorizer
	anAuthorizer.MachineAgent = true
	anAuthorizer.EnvironManager = false
	anAuthorizer.Tag = s.machines[0].Tag()
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.IsNil)
	c.Assert(aProvisioner, gc.NotNil)

	args := params.MachineContainersParams{
		Params: []params.MachineContainers{{
			MachineTag:     "machine-0",
			ContainerTypes: []instance.ContainerType{instance.LXC},
		}, {
			MachineTag:     "machine-1",
			ContainerTypes: []instance.ContainerType{instance.LXC},
		}, {
			MachineTag:     "machine-42",
			ContainerTypes: []instance.ContainerType{instance.LXC},
		},
		},
	}
	// Only machine 0 can have it's containers updated.
	results, err := aProvisioner.SetSupportedContainers(args)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutStateServerSuite) TestSupportsNoContainers(c *gc.C) {
	args := params.MachineContainersParams{
		Params: []params.MachineContainers{
			{
				MachineTag: "machine-0",
			},
		},
	}
	results, err := s.provisioner.SetSupportedContainers(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	m0, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	containers, ok := m0.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, []instance.ContainerType{})
}

var _ = gc.Suite(&withStateServerSuite{})

type withStateServerSuite struct {
	provisionerSuite
}

func (s *withStateServerSuite) SetUpTest(c *gc.C) {
	s.provisionerSuite.setUpTest(c, true)
}

func (s *withStateServerSuite) TestAPIAddresses(c *gc.C) {
	addrs, err := s.State.APIAddresses()
	c.Assert(err, gc.IsNil)

	result, err := s.provisioner.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResult{
		Result: addrs,
	})
}

func (s *withStateServerSuite) TestStateAddresses(c *gc.C) {
	addresses, err := s.State.Addresses()
	c.Assert(err, gc.IsNil)

	result, err := s.provisioner.StateAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResult{
		Result: addresses,
	})
}

func (s *withStateServerSuite) TestCACert(c *gc.C) {
	result := s.provisioner.CACert()
	c.Assert(result, gc.DeepEquals, params.BytesResult{
		Result: s.State.CACert(),
	})
}
