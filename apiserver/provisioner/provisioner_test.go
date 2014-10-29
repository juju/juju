// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/provisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
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
	for i := 0; i < 5; i++ {
		machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Check(err, gc.IsNil)
		s.machines = append(s.machines, machine)
	}

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming we logged in as the environment manager.
	s.authorizer = apiservertesting.FakeAuthorizer{
		EnvironManager: true,
	}

	// Create the resource registry separately to track invocations to
	// Register, and to register the root for tools URLs.
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
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: s.machines[0].Tag().String(), Password: "xxx0-1234567890123457890"},
			{Tag: s.machines[1].Tag().String(), Password: "xxx1-1234567890123457890"},
			{Tag: s.machines[2].Tag().String(), Password: "xxx2-1234567890123457890"},
			{Tag: s.machines[3].Tag().String(), Password: "xxx3-1234567890123457890"},
			{Tag: s.machines[4].Tag().String(), Password: "xxx4-1234567890123457890"},
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
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: s.machines[1].Tag().String(), Password: "xxx1"},
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
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: containers[0].Tag().String()},
		{Tag: containers[1].Tag().String()},
		{Tag: containers[2].Tag().String()},
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
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
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
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	result, err = s.provisioner.Life(params.Entities{
		Entities: []params.Entity{
			{Tag: s.machines[1].Tag().String()},
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
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
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
	err = s.machines[2].Refresh()
	c.Assert(err, gc.IsNil)
	s.assertLife(c, 2, state.Alive)
}

func (s *withoutStateServerSuite) TestSetStatus(c *gc.C) {
	err := s.machines[0].SetStatus(state.StatusStarted, "blah", nil)
	c.Assert(err, gc.IsNil)
	err = s.machines[1].SetStatus(state.StatusStopped, "foo", nil)
	c.Assert(err, gc.IsNil)
	err = s.machines[2].SetStatus(state.StatusError, "not really", nil)
	c.Assert(err, gc.IsNil)

	args := params.SetStatus{
		Entities: []params.EntityStatus{
			{Tag: s.machines[0].Tag().String(), Status: params.StatusError, Info: "not really",
				Data: map[string]interface{}{"foo": "bar"}},
			{Tag: s.machines[1].Tag().String(), Status: params.StatusStopped, Info: "foobar"},
			{Tag: s.machines[2].Tag().String(), Status: params.StatusStarted, Info: "again"},
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
	s.assertStatus(c, 0, state.StatusError, "not really", map[string]interface{}{"foo": "bar"})
	s.assertStatus(c, 1, state.StatusStopped, "foobar", map[string]interface{}{})
	s.assertStatus(c, 2, state.StatusStarted, "again", map[string]interface{}{})
}

func (s *withoutStateServerSuite) TestMachinesWithTransientErrors(c *gc.C) {
	err := s.machines[0].SetStatus(state.StatusStarted, "blah", nil)
	c.Assert(err, gc.IsNil)
	err = s.machines[1].SetStatus(state.StatusError, "transient error",
		map[string]interface{}{"transient": true, "foo": "bar"})
	c.Assert(err, gc.IsNil)
	err = s.machines[2].SetStatus(state.StatusError, "error", map[string]interface{}{"transient": false})
	c.Assert(err, gc.IsNil)
	err = s.machines[3].SetStatus(state.StatusError, "error", nil)
	c.Assert(err, gc.IsNil)
	// Machine 4 is provisioned but error not reset yet.
	err = s.machines[4].SetStatus(state.StatusError, "transient error",
		map[string]interface{}{"transient": true, "foo": "bar"})
	c.Assert(err, gc.IsNil)
	hwChars := instance.MustParseHardware("arch=i386", "mem=4G")
	err = s.machines[4].SetProvisioned("i-am", "fake_nonce", &hwChars)
	c.Assert(err, gc.IsNil)

	result, err := s.provisioner.MachinesWithTransientErrors()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Id: "1", Life: "alive", Status: "error", Info: "transient error",
				Data: map[string]interface{}{"transient": true, "foo": "bar"}},
		},
	})
}

func (s *withoutStateServerSuite) TestMachinesWithTransientErrorsPermission(c *gc.C) {
	// Machines where there's permission issues are omitted.
	anAuthorizer := s.authorizer
	anAuthorizer.EnvironManager = false
	anAuthorizer.Tag = names.NewMachineTag("1")
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources,
		anAuthorizer)
	err = s.machines[0].SetStatus(state.StatusStarted, "blah", nil)
	c.Assert(err, gc.IsNil)
	err = s.machines[1].SetStatus(state.StatusError, "transient error",
		map[string]interface{}{"transient": true, "foo": "bar"})
	c.Assert(err, gc.IsNil)
	err = s.machines[2].SetStatus(state.StatusError, "error", map[string]interface{}{"transient": false})
	c.Assert(err, gc.IsNil)
	err = s.machines[3].SetStatus(state.StatusError, "error", nil)
	c.Assert(err, gc.IsNil)

	result, err := aProvisioner.MachinesWithTransientErrors()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Id: "1", Life: "alive", Status: "error", Info: "transient error",
				Data: map[string]interface{}{"transient": true, "foo": "bar"}},
		},
	})
}

func (s *withoutStateServerSuite) TestEnsureDead(c *gc.C) {
	err := s.machines[1].EnsureDead()
	c.Assert(err, gc.IsNil)
	s.assertLife(c, 0, state.Alive)
	s.assertLife(c, 1, state.Dead)
	s.assertLife(c, 2, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
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

func (s *withoutStateServerSuite) assertStatus(c *gc.C, index int, expectStatus state.Status, expectInfo string,
	expectData map[string]interface{}) {

	status, info, data, err := s.machines[index].Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, expectStatus)
	c.Assert(info, gc.Equals, expectInfo)
	c.Assert(data, gc.DeepEquals, expectData)
}

func (s *withoutStateServerSuite) TestWatchContainers(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.WatchContainers{Params: []params.WatchContainer{
		{MachineTag: s.machines[0].Tag().String(), ContainerType: string(instance.LXC)},
		{MachineTag: s.machines[1].Tag().String(), ContainerType: string(instance.KVM)},
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
		{MachineTag: s.machines[0].Tag().String()},
		{MachineTag: s.machines[1].Tag().String()},
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
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.EnvironManager = false
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources,
		anAuthorizer)
	c.Assert(err, gc.IsNil)
	s.AssertEnvironConfig(c, aProvisioner, commontesting.NoSecrets)
}

func (s *withoutStateServerSuite) TestStatus(c *gc.C) {
	err := s.machines[0].SetStatus(state.StatusStarted, "blah", nil)
	c.Assert(err, gc.IsNil)
	err = s.machines[1].SetStatus(state.StatusStopped, "foo", nil)
	c.Assert(err, gc.IsNil)
	err = s.machines[2].SetStatus(state.StatusError, "not really", map[string]interface{}{"foo": "bar"})
	c.Assert(err, gc.IsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "service-bar"},
	}}
	result, err := s.provisioner.Status(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Status: params.StatusStarted, Info: "blah", Data: map[string]interface{}{}},
			{Status: params.StatusStopped, Info: "foo", Data: map[string]interface{}{}},
			{Status: params.StatusError, Info: "not really", Data: map[string]interface{}{"foo": "bar"}},
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
		{Tag: s.machines[0].Tag().String()},
		{Tag: foobarMachine.Tag().String()},
		{Tag: s.machines[2].Tag().String()},
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

func (s *withoutStateServerSuite) TestDistributionGroup(c *gc.C) {
	addUnits := func(name string, machines ...*state.Machine) (units []*state.Unit) {
		svc := s.AddTestingService(c, name, s.AddTestingCharm(c, name))
		for _, m := range machines {
			unit, err := svc.AddUnit()
			c.Assert(err, gc.IsNil)
			err = unit.AssignToMachine(m)
			c.Assert(err, gc.IsNil)
			units = append(units, unit)
		}
		return units
	}
	setProvisioned := func(id string) {
		m, err := s.State.Machine(id)
		c.Assert(err, gc.IsNil)
		err = m.SetProvisioned(instance.Id("machine-"+id+"-inst"), "nonce", nil)
		c.Assert(err, gc.IsNil)
	}

	mysqlUnit := addUnits("mysql", s.machines[0], s.machines[3])[0]
	wordpressUnits := addUnits("wordpress", s.machines[0], s.machines[1], s.machines[2])

	// Unassign wordpress/1 from machine-1.
	// The unit should not show up in the results.
	err := wordpressUnits[1].UnassignFromMachine()
	c.Assert(err, gc.IsNil)

	// Provision machines 1, 2 and 3. Machine-0 remains
	// unprovisioned, and machine-1 has no units, and so
	// neither will show up in the results.
	setProvisioned("1")
	setProvisioned("2")
	setProvisioned("3")

	// Add a few state servers, provision two of them.
	_, err = s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, gc.IsNil)
	setProvisioned("5")
	setProvisioned("7")

	// Create a logging service, subordinate to mysql.
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("mysql", "logging")
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	ru, err := rel.Unit(mysqlUnit)
	c.Assert(err, gc.IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, gc.IsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: s.machines[3].Tag().String()},
		{Tag: "machine-5"},
	}}
	result, err := s.provisioner.DistributionGroup(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.DistributionGroupResults{
		Results: []params.DistributionGroupResult{
			{Result: []instance.Id{"machine-2-inst", "machine-3-inst"}},
			{Result: []instance.Id{}},
			{Result: []instance.Id{"machine-2-inst"}},
			{Result: []instance.Id{"machine-3-inst"}},
			{Result: []instance.Id{"machine-5-inst", "machine-7-inst"}},
		},
	})
}

func (s *withoutStateServerSuite) TestDistributionGroupEnvironManagerAuth(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxc-99"},
		{Tag: "unit-foo-0"},
		{Tag: "service-bar"},
	}}
	result, err := s.provisioner.DistributionGroup(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.DistributionGroupResults{
		Results: []params.DistributionGroupResult{
			// environ manager may access any top-level machines.
			{Result: []instance.Id{}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			// only a machine agent for the container or its
			// parent may access it.
			{Error: apiservertesting.ErrUnauthorized},
			// non-machines always unauthorized
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutStateServerSuite) TestDistributionGroupMachineAgentAuth(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.EnvironManager = false
	provisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, gc.IsNil)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxc-99"},
		{Tag: "machine-1-lxc-99"},
		{Tag: "machine-1-lxc-99-lxc-100"},
	}}
	result, err := provisioner.DistributionGroup(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.DistributionGroupResults{
		Results: []params.DistributionGroupResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: []instance.Id{}},
			{Error: apiservertesting.ErrUnauthorized},
			// only a machine agent for the container or its
			// parent may access it.
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError("machine 1/lxc/99")},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutStateServerSuite) TestProvisioningInfo(c *gc.C) {
	cons := constraints.MustParse("cpu-cores=123 mem=8G networks=^net3,^net4")
	template := state.MachineTemplate{
		Series:            "quantal",
		Jobs:              []state.MachineJob{state.JobHostUnits},
		Constraints:       cons,
		Placement:         "valid",
		RequestedNetworks: []string{"net1", "net2"},
	}
	placementMachine, err := s.State.AddOneMachine(template)
	c.Assert(err, gc.IsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: placementMachine.Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "service-bar"},
	}}
	result, err := s.provisioner.ProvisioningInfo(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ProvisioningInfoResults{
		Results: []params.ProvisioningInfoResult{
			{Result: &params.ProvisioningInfo{
				Series:   "quantal",
				Networks: []string{},
				Jobs:     []params.MachineJob{params.JobHostUnits},
			}},
			{Result: &params.ProvisioningInfo{
				Series:      "quantal",
				Constraints: template.Constraints,
				Placement:   template.Placement,
				Networks:    template.RequestedNetworks,
				Jobs:        []params.MachineJob{params.JobHostUnits},
			}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutStateServerSuite) TestProvisioningInfoPermissions(c *gc.C) {
	// Login as a machine agent for machine 0.
	anAuthorizer := s.authorizer
	anAuthorizer.EnvironManager = false
	anAuthorizer.Tag = s.machines[0].Tag()
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.IsNil)
	c.Assert(aProvisioner, gc.NotNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[0].Tag().String() + "-lxc-0"},
		{Tag: "machine-42"},
		{Tag: s.machines[1].Tag().String()},
		{Tag: "service-bar"},
	}}

	// Only machine 0 and containers therein can be accessed.
	results, err := aProvisioner.ProvisioningInfo(args)
	c.Assert(results, gc.DeepEquals, params.ProvisioningInfoResults{
		Results: []params.ProvisioningInfoResult{
			{Result: &params.ProvisioningInfo{
				Series:   "quantal",
				Networks: []string{},
				Jobs:     []params.MachineJob{params.JobHostUnits},
			}},
			{Error: apiservertesting.NotFoundError("machine 0/lxc/0")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutStateServerSuite) TestConstraints(c *gc.C) {
	// Add a machine with some constraints.
	cons := constraints.MustParse("cpu-cores=123", "mem=8G", "networks=net3,^net4")
	template := state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: cons,
	}
	consMachine, err := s.State.AddOneMachine(template)
	c.Assert(err, gc.IsNil)

	machine0Constraints, err := s.machines[0].Constraints()
	c.Assert(err, gc.IsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: consMachine.Tag().String()},
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

func (s *withoutStateServerSuite) TestRequestedNetworks(c *gc.C) {
	// Add a machine with some requested networks.
	template := state.MachineTemplate{
		Series:            "quantal",
		Jobs:              []state.MachineJob{state.JobHostUnits},
		RequestedNetworks: []string{"net1", "net2"},
	}
	netsMachine, err := s.State.AddOneMachine(template)
	c.Assert(err, gc.IsNil)

	networksMachine0, err := s.machines[0].RequestedNetworks()
	c.Assert(err, gc.IsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: netsMachine.Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "service-bar"},
	}}
	result, err := s.provisioner.RequestedNetworks(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.RequestedNetworksResults{
		Results: []params.RequestedNetworkResult{
			{
				Networks: networksMachine0,
			},
			{
				Networks: template.RequestedNetworks,
			},
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
		{Tag: s.machines[0].Tag().String(), InstanceId: "i-was", Nonce: "fake_nonce", Characteristics: nil},
		{Tag: s.machines[1].Tag().String(), InstanceId: "i-will", Nonce: "fake_nonce", Characteristics: &hwChars},
		{Tag: s.machines[2].Tag().String(), InstanceId: "i-am-too", Nonce: "fake", Characteristics: nil},
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

func (s *withoutStateServerSuite) TestSetInstanceInfo(c *gc.C) {
	// Provision machine 0 first.
	hwChars := instance.MustParseHardware("arch=i386", "mem=4G")
	err := s.machines[0].SetInstanceInfo("i-am", "fake_nonce", &hwChars, nil, nil)
	c.Assert(err, gc.IsNil)

	networks := []params.Network{{
		Tag:        "network-net1",
		ProviderId: "net1",
		CIDR:       "0.1.2.0/24",
		VLANTag:    0,
	}, {
		Tag:        "network-vlan42",
		ProviderId: "vlan42",
		CIDR:       "0.2.2.0/24",
		VLANTag:    42,
	}, {
		Tag:        "network-vlan69",
		ProviderId: "vlan69",
		CIDR:       "0.3.2.0/24",
		VLANTag:    69,
	}, {
		Tag:        "network-vlan42", // duplicated; ignored
		ProviderId: "vlan42",
		CIDR:       "0.2.2.0/24",
		VLANTag:    42,
	}}
	ifaces := []params.NetworkInterface{{
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		NetworkTag:    "network-net1",
		InterfaceName: "eth0",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		NetworkTag:    "network-net1",
		InterfaceName: "eth1",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		NetworkTag:    "network-vlan42",
		InterfaceName: "eth1.42",
		IsVirtual:     true,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		NetworkTag:    "network-vlan69",
		InterfaceName: "eth0.69",
		IsVirtual:     true,
		Disabled:      true,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1", // duplicated mac+net; ignored
		NetworkTag:    "network-vlan42",
		InterfaceName: "eth2",
		IsVirtual:     true,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f2",
		NetworkTag:    "network-net1",
		InterfaceName: "eth1", // duplicated name+machine id; ignored for machine 1.
		IsVirtual:     false,
	}}
	args := params.InstancesInfo{Machines: []params.InstanceInfo{{
		Tag:        s.machines[0].Tag().String(),
		InstanceId: "i-was",
		Nonce:      "fake_nonce",
	}, {
		Tag:             s.machines[1].Tag().String(),
		InstanceId:      "i-will",
		Nonce:           "fake_nonce",
		Characteristics: &hwChars,
		Networks:        networks,
		Interfaces:      ifaces,
	}, {
		Tag:             s.machines[2].Tag().String(),
		InstanceId:      "i-am-too",
		Nonce:           "fake",
		Characteristics: nil,
		Networks:        networks,
		Interfaces:      ifaces,
	},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "service-bar"},
	}}
	result, err := s.provisioner.SetInstanceInfo(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{
				Message: `aborted instance "i-was": cannot set instance data for machine "0": already set`,
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
	ifacesMachine1, err := s.machines[1].NetworkInterfaces()
	c.Assert(err, gc.IsNil)
	c.Assert(ifacesMachine1, gc.HasLen, 4)
	actual := make([]params.NetworkInterface, len(ifacesMachine1))
	for i, iface := range ifacesMachine1 {
		actual[i].InterfaceName = iface.InterfaceName()
		actual[i].NetworkTag = iface.NetworkTag().String()
		actual[i].MACAddress = iface.MACAddress()
		actual[i].IsVirtual = iface.IsVirtual()
		actual[i].Disabled = iface.IsDisabled()
		c.Check(iface.MachineId(), gc.Equals, s.machines[1].Id())
		c.Check(iface.MachineTag(), gc.Equals, s.machines[1].Tag())
	}
	c.Assert(actual, jc.SameContents, ifaces[:4])
	ifacesMachine2, err := s.machines[2].NetworkInterfaces()
	c.Assert(err, gc.IsNil)
	c.Assert(ifacesMachine2, gc.HasLen, 1)
	c.Assert(ifacesMachine2[0].InterfaceName(), gc.Equals, ifaces[5].InterfaceName)
	c.Assert(ifacesMachine2[0].MACAddress(), gc.Equals, ifaces[5].MACAddress)
	c.Assert(ifacesMachine2[0].NetworkTag().String(), gc.Equals, ifaces[5].NetworkTag)
	c.Assert(ifacesMachine2[0].MachineId(), gc.Equals, s.machines[2].Id())
	for i, _ := range networks {
		if i == 3 {
			// Last one was ignored, so don't check.
			break
		}
		tag, err := names.ParseNetworkTag(networks[i].Tag)
		c.Assert(err, gc.IsNil)
		networkName := tag.Id()
		network, err := s.State.Network(networkName)
		c.Assert(err, gc.IsNil)
		c.Check(network.Name(), gc.Equals, networkName)
		c.Check(network.ProviderId(), gc.Equals, networks[i].ProviderId)
		c.Check(network.Tag().String(), gc.Equals, networks[i].Tag)
		c.Check(network.VLANTag(), gc.Equals, networks[i].VLANTag)
		c.Check(network.CIDR(), gc.Equals, networks[i].CIDR)
	}
}

func (s *withoutStateServerSuite) TestInstanceId(c *gc.C) {
	// Provision 2 machines first.
	err := s.machines[0].SetProvisioned("i-am", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	hwChars := instance.MustParseHardware("arch=i386", "mem=4G")
	err = s.machines[1].SetProvisioned("i-am-not", "fake_nonce", &hwChars)
	c.Assert(err, gc.IsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
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

	got, err := s.provisioner.WatchEnvironMachines()
	c.Assert(err, gc.IsNil)
	want := params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{"0", "1", "2", "3", "4"},
	}
	c.Assert(got.StringsWatcherId, gc.Equals, want.StringsWatcherId)
	c.Assert(got.Changes, jc.SameContents, want.Changes)

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
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.EnvironManager = false
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.IsNil)

	result, err := aProvisioner.WatchEnvironMachines()
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{})
}

func (s *withoutStateServerSuite) TestContainerManagerConfig(c *gc.C) {
	args := params.ContainerManagerConfigParams{Type: instance.KVM}
	results, err := s.provisioner.ContainerManagerConfig(args)
	c.Check(err, gc.IsNil)
	c.Assert(results.ManagerConfig, gc.DeepEquals, map[string]string{
		container.ConfigName: "juju",
	})
}

func (s *withoutStateServerSuite) TestContainerConfig(c *gc.C) {
	attrs := map[string]interface{}{
		"http-proxy": "http://proxy.example.com:9000",
	}
	err := s.State.UpdateEnvironConfig(attrs, nil, nil)
	c.Assert(err, gc.IsNil)
	expectedProxy := proxy.Settings{
		Http: "http://proxy.example.com:9000",
	}

	results, err := s.provisioner.ContainerConfig()
	c.Check(err, gc.IsNil)
	c.Check(results.UpdateBehavior, gc.Not(gc.IsNil))
	c.Check(results.ProviderType, gc.Equals, "dummy")
	c.Check(results.AuthorizedKeys, gc.Equals, s.Environ.Config().AuthorizedKeys())
	c.Check(results.SSLHostnameVerification, jc.IsTrue)
	c.Check(results.Proxy, gc.DeepEquals, expectedProxy)
	c.Check(results.AptProxy, gc.DeepEquals, expectedProxy)
	c.Check(results.PreferIPv6, jc.IsTrue)
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
	hostPorts := [][]network.HostPort{{{
		Address: network.NewAddress("0.1.2.3", network.ScopeUnknown),
		Port:    1234,
	}}}

	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, gc.IsNil)

	result, err := s.provisioner.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResult{
		Result: []string{"0.1.2.3:1234"},
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
		Result: []byte(s.State.CACert()),
	})
}

func (s *withoutStateServerSuite) TestWatchMachineErrorRetry(c *gc.C) {
	s.PatchValue(&provisioner.ErrorRetryWaitDelay, 2*coretesting.ShortWait)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	_, err := s.provisioner.WatchMachineErrorRetry()
	c.Assert(err, gc.IsNil)

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, s.State, resource.(state.NotifyWatcher))
	wc.AssertNoChange()

	// We should now get a time triggered change.
	wc.AssertOneChange()

	// Make sure WatchMachineErrorRetry fails with a machine agent login.
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.EnvironManager = false
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.IsNil)

	result, err := aProvisioner.WatchMachineErrorRetry()
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{})
}

func (s *withoutStateServerSuite) TestFindTools(c *gc.C) {
	args := params.FindToolsParams{
		MajorVersion: -1,
		MinorVersion: -1,
	}
	result, err := s.provisioner.FindTools(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.List, gc.Not(gc.HasLen), 0)
	for _, tools := range result.List {
		url := fmt.Sprintf("https://%s/environment/90168e4c-2f10-4e9c-83c2-feedfacee5a9/tools/%s", s.APIState.Addr(), tools.Version)
		c.Assert(tools.URL, gc.Equals, url)
	}
}
