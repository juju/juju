// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	commontesting "launchpad.net/juju-core/state/apiserver/common/testing"
	"launchpad.net/juju-core/state/apiserver/firewaller"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type firewallerSuite struct {
	testing.JujuConnSuite
	*commontesting.EnvironWatcherTest

	machines []*state.Machine
	service  *state.Service
	charm    *state.Charm
	units    []*state.Unit

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources
	firewaller *firewaller.FirewallerAPI
}

var _ = gc.Suite(&firewallerSuite{})

func (s *firewallerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Reset previous machines and units (if any) and create 3
	// machines for the tests.
	s.machines = nil
	s.units = nil
	// Note that the specific machine ids allocated are assumed
	// to be numerically consecutive from zero.
	for i := 0; i <= 2; i++ {
		machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Check(err, gc.IsNil)
		s.machines = append(s.machines, machine)
	}
	// Create a service and three units for these machines.
	s.charm = s.AddTestingCharm(c, "wordpress")
	s.service = s.AddTestingService(c, "wordpress", s.charm)
	// Add the rest of the units and assign them.
	for i := 0; i <= 2; i++ {
		unit, err := s.service.AddUnit()
		c.Check(err, gc.IsNil)
		err = unit.AssignToMachine(s.machines[i])
		c.Check(err, gc.IsNil)
		s.units = append(s.units, unit)
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

	// Create a firewaller API for the machine.
	firewallerAPI, err := firewaller.NewFirewallerAPI(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, gc.IsNil)
	s.firewaller = firewallerAPI
	s.EnvironWatcherTest = commontesting.NewEnvironWatcherTest(s.firewaller, s.State, s.resources, commontesting.HasSecrets)
}

func (s *firewallerSuite) TestFirewallerFailsWithNonEnvironManagerUser(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.MachineAgent = true
	anAuthorizer.EnvironManager = false
	aFirewaller, err := firewaller.NewFirewallerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(aFirewaller, gc.IsNil)
}

func (s *firewallerSuite) TestLife(c *gc.C) {
	// Unassign unit 1 from its machine, so we can change its life cycle.
	err := s.units[1].UnassignFromMachine()
	c.Assert(err, gc.IsNil)

	err = s.machines[1].EnsureDead()
	c.Assert(err, gc.IsNil)
	s.assertLife(c, 0, state.Alive)
	s.assertLife(c, 1, state.Dead)
	s.assertLife(c, 2, state.Alive)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag()},
		{Tag: s.machines[1].Tag()},
		{Tag: s.machines[2].Tag()},
	}})
	result, err := s.firewaller.Life(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "alive"},
			{Life: "dead"},
			{Life: "alive"},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.NotFoundError(`unit "foo/0"`)},
			{Error: apiservertesting.NotFoundError(`service "bar"`)},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Remove a machine and make sure it's detected.
	err = s.machines[1].Remove()
	c.Assert(err, gc.IsNil)
	err = s.machines[1].Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	args = params.Entities{
		Entities: []params.Entity{
			{Tag: s.machines[1].Tag()},
		},
	}
	result, err = s.firewaller.Life(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.NotFoundError("machine 1")},
		},
	})
}

func (s *firewallerSuite) TestInstanceId(c *gc.C) {
	// Provision 2 machines first.
	err := s.machines[0].SetProvisioned("i-am", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	hwChars := instance.MustParseHardware("arch=i386", "mem=4G")
	err = s.machines[1].SetProvisioned("i-am-not", "fake_nonce", &hwChars)
	c.Assert(err, gc.IsNil)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag()},
		{Tag: s.machines[1].Tag()},
		{Tag: s.machines[2].Tag()},
		{Tag: s.service.Tag()},
		{Tag: s.units[2].Tag()},
	}})
	result, err := s.firewaller.InstanceId(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "i-am"},
			{Result: "i-am-not"},
			{Error: apiservertesting.NotProvisionedError("2")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *firewallerSuite) TestWatchEnvironMachines(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	result, err := s.firewaller.WatchEnvironMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResult{
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
}

func (s *firewallerSuite) TestWatch(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag()},
		{Tag: s.service.Tag()},
		{Tag: s.units[0].Tag()},
	}})
	result, err := s.firewaller.Watch(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{NotifyWatcherId: "1"},
			{NotifyWatcherId: "2"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError(`unit "foo/0"`)},
			{Error: apiservertesting.NotFoundError(`service "bar"`)},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop when done.
	c.Assert(s.resources.Count(), gc.Equals, 2)
	c.Assert(result.Results[1].NotifyWatcherId, gc.Equals, "1")
	watcher1 := s.resources.Get("1")
	defer statetesting.AssertStop(c, watcher1)
	c.Assert(result.Results[2].NotifyWatcherId, gc.Equals, "2")
	watcher2 := s.resources.Get("2")
	defer statetesting.AssertStop(c, watcher2)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc1 := statetesting.NewNotifyWatcherC(c, s.State, watcher1.(state.NotifyWatcher))
	wc1.AssertNoChange()
	wc2 := statetesting.NewNotifyWatcherC(c, s.State, watcher2.(state.NotifyWatcher))
	wc2.AssertNoChange()
}

func (s *firewallerSuite) TestWatchUnits(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag()},
		{Tag: s.service.Tag()},
		{Tag: s.units[0].Tag()},
	}})
	result, err := s.firewaller.WatchUnits(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Changes: []string{"wordpress/0"}, StringsWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	c.Assert(result.Results[0].StringsWatcherId, gc.Equals, "1")
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *firewallerSuite) TestGetExposed(c *gc.C) {
	// Set the service to exposed first.
	err := s.service.SetExposed()
	c.Assert(err, gc.IsNil)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.service.Tag()},
	}})
	result, err := s.firewaller.GetExposed(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Result: true},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError(`service "bar"`)},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now reset the exposed flag for the service and check again.
	err = s.service.ClearExposed()
	c.Assert(err, gc.IsNil)

	args = params.Entities{Entities: []params.Entity{
		{Tag: s.service.Tag()},
	}}
	result, err = s.firewaller.GetExposed(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Result: false},
		},
	})
}

func (s *firewallerSuite) TestOpenedPorts(c *gc.C) {
	// Open some ports on two of the units.
	err := s.units[0].OpenPort("foo", 1234)
	c.Assert(err, gc.IsNil)
	err = s.units[0].OpenPort("bar", 4321)
	c.Assert(err, gc.IsNil)
	err = s.units[2].OpenPort("baz", 1111)
	c.Assert(err, gc.IsNil)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.units[0].Tag()},
		{Tag: s.units[1].Tag()},
		{Tag: s.units[2].Tag()},
	}})
	result, err := s.firewaller.OpenedPorts(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.PortsResults{
		Results: []params.PortsResult{
			{Ports: []instance.Port{{"bar", 4321}, {"foo", 1234}}},
			{Ports: []instance.Port{}},
			{Ports: []instance.Port{{"baz", 1111}}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError(`unit "foo/0"`)},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now close unit 2's port and check again.
	err = s.units[2].ClosePort("baz", 1111)
	c.Assert(err, gc.IsNil)

	args = params.Entities{Entities: []params.Entity{
		{Tag: s.units[2].Tag()},
	}}
	result, err = s.firewaller.OpenedPorts(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.PortsResults{
		Results: []params.PortsResult{
			{Ports: []instance.Port{}},
		},
	})
}

func (s *firewallerSuite) TestGetAssignedMachine(c *gc.C) {
	// Unassign a unit first.
	err := s.units[2].UnassignFromMachine()
	c.Assert(err, gc.IsNil)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.units[0].Tag()},
		{Tag: s.units[1].Tag()},
		{Tag: s.units[2].Tag()},
	}})
	result, err := s.firewaller.GetAssignedMachine(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: s.machines[0].Tag()},
			{Result: s.machines[1].Tag()},
			{Error: apiservertesting.NotAssignedError("wordpress/2")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError(`unit "foo/0"`)},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now reset assign unit 2 again and check.
	err = s.units[2].AssignToMachine(s.machines[0])
	c.Assert(err, gc.IsNil)

	args = params.Entities{Entities: []params.Entity{
		{Tag: s.units[2].Tag()},
	}}
	result, err = s.firewaller.GetAssignedMachine(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: s.machines[0].Tag()},
		},
	})
}

func (s *firewallerSuite) assertLife(c *gc.C, index int, expectLife state.Life) {
	err := s.machines[index].Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.machines[index].Life(), gc.Equals, expectLife)
}

var commonFakeEntities = []params.Entity{
	{Tag: "machine-42"},
	{Tag: "unit-foo-0"},
	{Tag: "service-bar"},
	{Tag: "user-foo"},
	{Tag: "foo-bar"},
	{Tag: ""},
}

func addFakeEntities(actual params.Entities) params.Entities {
	for _, entity := range commonFakeEntities {
		actual.Entities = append(actual.Entities, entity)
	}
	return actual
}
