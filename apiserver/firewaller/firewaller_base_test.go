// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

// firewallerBaseSuite implements common testing suite for all API
// versions. It's not intended to be used directly or registered as a
// suite, but embedded.
type firewallerBaseSuite struct {
	testing.JujuConnSuite

	machines []*state.Machine
	service  *state.Service
	charm    *state.Charm
	units    []*state.Unit

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources
}

func (s *firewallerBaseSuite) setUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Reset previous machines and units (if any) and create 3
	// machines for the tests.
	s.machines = nil
	s.units = nil
	// Note that the specific machine ids allocated are assumed
	// to be numerically consecutive from zero.
	for i := 0; i <= 2; i++ {
		machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Check(err, jc.ErrorIsNil)
		s.machines = append(s.machines, machine)
	}
	// Create a service and three units for these machines.
	s.charm = s.AddTestingCharm(c, "wordpress")
	s.service = s.AddTestingService(c, "wordpress", s.charm)
	// Add the rest of the units and assign them.
	for i := 0; i <= 2; i++ {
		unit, err := s.service.AddUnit()
		c.Check(err, jc.ErrorIsNil)
		err = unit.AssignToMachine(s.machines[i])
		c.Check(err, jc.ErrorIsNil)
		s.units = append(s.units, unit)
	}

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming we logged in as the environment manager.
	s.authorizer = apiservertesting.FakeAuthorizer{
		EnvironManager: true,
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
}

func (s *firewallerBaseSuite) testFirewallerFailsWithNonEnvironManagerUser(
	c *gc.C,
	factory func(_ *state.State, _ *common.Resources, _ common.Authorizer) error,
) {
	anAuthorizer := s.authorizer
	anAuthorizer.EnvironManager = false
	err := factory(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *firewallerBaseSuite) testLife(
	c *gc.C,
	facade interface {
		Life(args params.Entities) (params.LifeResults, error)
	},
) {
	// Unassign unit 1 from its machine, so we can change its life cycle.
	err := s.units[1].UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)

	err = s.machines[1].EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	s.assertLife(c, 0, state.Alive)
	s.assertLife(c, 1, state.Dead)
	s.assertLife(c, 2, state.Alive)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
	}})
	result, err := facade.Life(args)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	err = s.machines[1].Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	args = params.Entities{
		Entities: []params.Entity{
			{Tag: s.machines[1].Tag().String()},
		},
	}
	result, err = facade.Life(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.NotFoundError("machine 1")},
		},
	})
}

func (s *firewallerBaseSuite) testInstanceId(
	c *gc.C,
	facade interface {
		InstanceId(args params.Entities) (params.StringResults, error)
	},
) {
	// Provision 2 machines first.
	err := s.machines[0].SetProvisioned("i-am", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	hwChars := instance.MustParseHardware("arch=i386", "mem=4G")
	err = s.machines[1].SetProvisioned("i-am-not", "fake_nonce", &hwChars)
	c.Assert(err, jc.ErrorIsNil)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: s.service.Tag().String()},
		{Tag: s.units[2].Tag().String()},
	}})
	result, err := facade.InstanceId(args)
	c.Assert(err, jc.ErrorIsNil)
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

func (s *firewallerBaseSuite) testWatchEnvironMachines(
	c *gc.C,
	facade interface {
		WatchEnvironMachines() (params.StringsWatchResult, error)
	},
) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	got, err := facade.WatchEnvironMachines()
	c.Assert(err, jc.ErrorIsNil)
	want := params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{"0", "1", "2"},
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
}

const (
	canWatchUnits    = true
	cannotWatchUnits = false
)

func (s *firewallerBaseSuite) testWatch(
	c *gc.C,
	facade interface {
		Watch(args params.Entities) (params.NotifyWatchResults, error)
	},
	allowUnits bool,
) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.service.Tag().String()},
		{Tag: s.units[0].Tag().String()},
	}})
	result, err := facade.Watch(args)
	c.Assert(err, jc.ErrorIsNil)
	if allowUnits {
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
	} else {
		c.Assert(result, jc.DeepEquals, params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{
				{Error: apiservertesting.ErrUnauthorized},
				{NotifyWatcherId: "1"},
				{Error: apiservertesting.ErrUnauthorized},
				{Error: apiservertesting.ErrUnauthorized},
				{Error: apiservertesting.ErrUnauthorized},
				{Error: apiservertesting.NotFoundError(`service "bar"`)},
				{Error: apiservertesting.ErrUnauthorized},
				{Error: apiservertesting.ErrUnauthorized},
				{Error: apiservertesting.ErrUnauthorized},
			},
		})
	}

	// Verify the resources were registered and stop when done.
	if allowUnits {
		c.Assert(s.resources.Count(), gc.Equals, 2)
	} else {
		c.Assert(s.resources.Count(), gc.Equals, 1)
	}
	c.Assert(result.Results[1].NotifyWatcherId, gc.Equals, "1")
	watcher1 := s.resources.Get("1")
	defer statetesting.AssertStop(c, watcher1)
	var watcher2 common.Resource
	if allowUnits {
		c.Assert(result.Results[2].NotifyWatcherId, gc.Equals, "2")
		watcher2 = s.resources.Get("2")
		defer statetesting.AssertStop(c, watcher2)
	}

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc1 := statetesting.NewNotifyWatcherC(c, s.State, watcher1.(state.NotifyWatcher))
	wc1.AssertNoChange()
	if allowUnits {
		wc2 := statetesting.NewNotifyWatcherC(c, s.State, watcher2.(state.NotifyWatcher))
		wc2.AssertNoChange()
	}
}

func (s *firewallerBaseSuite) testWatchUnits(
	c *gc.C,
	facade interface {
		WatchUnits(args params.Entities) (params.StringsWatchResults, error)
	},
) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.service.Tag().String()},
		{Tag: s.units[0].Tag().String()},
	}})
	result, err := facade.WatchUnits(args)
	c.Assert(err, jc.ErrorIsNil)
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

func (s *firewallerBaseSuite) testGetExposed(
	c *gc.C,
	facade interface {
		GetExposed(args params.Entities) (params.BoolResults, error)
	},
) {
	// Set the service to exposed first.
	err := s.service.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.service.Tag().String()},
	}})
	result, err := facade.GetExposed(args)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)

	args = params.Entities{Entities: []params.Entity{
		{Tag: s.service.Tag().String()},
	}}
	result, err = facade.GetExposed(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Result: false},
		},
	})
}

func (s *firewallerBaseSuite) testGetAssignedMachine(
	c *gc.C,
	facade interface {
		GetAssignedMachine(args params.Entities) (params.StringResults, error)
	},
) {
	// Unassign a unit first.
	err := s.units[2].UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.units[0].Tag().String()},
		{Tag: s.units[1].Tag().String()},
		{Tag: s.units[2].Tag().String()},
	}})
	result, err := facade.GetAssignedMachine(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: s.machines[0].Tag().String()},
			{Result: s.machines[1].Tag().String()},
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
	c.Assert(err, jc.ErrorIsNil)

	args = params.Entities{Entities: []params.Entity{
		{Tag: s.units[2].Tag().String()},
	}}
	result, err = facade.GetAssignedMachine(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: s.machines[0].Tag().String()},
		},
	})
}

func (s *firewallerBaseSuite) assertLife(c *gc.C, index int, expectLife state.Life) {
	err := s.machines[index].Refresh()
	c.Assert(err, jc.ErrorIsNil)
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
