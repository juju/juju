// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/instance"
	servicefactorytesting "github.com/juju/juju/domain/servicefactory/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

// firewallerBaseSuite implements common testing suite for all API
// versions. It's not intended to be used directly or registered as a
// suite, but embedded.
type firewallerBaseSuite struct {
	testing.ApiServerSuite

	machines    []*state.Machine
	application *state.Application
	charm       *state.Charm
	units       []*state.Unit
	relations   []*state.Relation

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources
}

func (s *firewallerBaseSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	// Reset previous machines and units (if any) and create 3
	// machines for the tests.
	s.machines = nil
	s.units = nil
	// Note that the specific machine ids allocated are assumed
	// to be numerically consecutive from zero.
	st := s.ControllerModel(c).State()
	for i := 0; i <= 2; i++ {
		machine, err := st.AddMachine(s.InstancePrechecker(c, st), state.UbuntuBase("12.10"), state.JobHostUnits)
		c.Check(err, jc.ErrorIsNil)
		s.machines = append(s.machines, machine)
	}
	// Create an application and three units for these machines.
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	s.charm = f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	s.application = f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: s.charm,
	})
	// Add the rest of the units and assign them.
	for i := 0; i <= 2; i++ {
		unit, err := s.application.AddUnit(state.AddUnitParams{})
		c.Check(err, jc.ErrorIsNil)
		err = unit.AssignToMachine(s.machines[i])
		c.Check(err, jc.ErrorIsNil)
		s.units = append(s.units, unit)
	}

	// Create a relation.
	f.MakeApplication(c, nil)
	eps, err := st.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)

	s.relations = make([]*state.Relation, 1)
	s.relations[0], err = st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming we logged in as the controller.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Controller: true,
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
}

func (s *firewallerBaseSuite) testFirewallerFailsWithNonControllerUser(
	c *gc.C,
	factory func(_ facade.ModelContext) error,
) {
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = false
	ctx := facadetest.ModelContext{
		Auth_:           anAuthorizer,
		Resources_:      s.resources,
		State_:          s.ControllerModel(c).State(),
		ServiceFactory_: servicefactorytesting.NewTestingServiceFactory(),
	}
	err := factory(ctx)
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *firewallerBaseSuite) testLife(
	c *gc.C,
	facade interface {
		Life(context.Context, params.Entities) (params.LifeResults, error)
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
		{Tag: s.relations[0].Tag().String()},
	}})
	result, err := facade.Life(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "alive"},
			{Life: "dead"},
			{Life: "alive"},
			{Life: "alive"},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.NotFoundError(`unit "foo/0"`)},
			{Error: apiservertesting.NotFoundError(`application "bar"`)},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Remove a machine and make sure it's detected.
	err = s.machines[1].Remove(testing.NewObjectStore(c, s.ControllerModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = s.machines[1].Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	args = params.Entities{
		Entities: []params.Entity{
			{Tag: s.machines[1].Tag().String()},
		},
	}
	result, err = facade.Life(context.Background(), args)
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
		InstanceId(context.Context, params.Entities) (params.StringResults, error)
	},
) {
	// Provision 2 machines first.
	err := s.machines[0].SetProvisioned("i-am", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	hwChars := instance.MustParseHardware("arch=s390x", "mem=4G")
	err = s.machines[1].SetProvisioned("i-am-not", "", "fake_nonce", &hwChars)
	c.Assert(err, jc.ErrorIsNil)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: s.application.Tag().String()},
		{Tag: s.units[2].Tag().String()},
	}})
	result, err := facade.InstanceId(context.Background(), args)
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

func (s *firewallerBaseSuite) testWatchModelMachines(
	c *gc.C,
	facade interface {
		WatchModelMachines(context.Context) (params.StringsWatchResult, error)
	},
) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	got, err := facade.WatchModelMachines(context.Background())
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
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

const (
	cannotWatchUnits = false
)

func (s *firewallerBaseSuite) testWatch(
	c *gc.C,
	watcher interface {
		Watch(context.Context, params.Entities) (params.NotifyWatchResults, error)
	},
	allowUnits bool,
) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.application.Tag().String()},
		{Tag: s.units[0].Tag().String()},
	}})
	result, err := watcher.Watch(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	if allowUnits {
		c.Assert(result, jc.DeepEquals, params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{
				{Error: apiservertesting.ErrUnauthorized},
				{NotifyWatcherId: "1"},
				{NotifyWatcherId: "2"},
				{Error: apiservertesting.ErrUnauthorized},
				{Error: apiservertesting.NotFoundError(`unit "foo/0"`)},
				{Error: apiservertesting.NotFoundError(`application "bar"`)},
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
				{Error: apiservertesting.NotFoundError(`application "bar"`)},
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
	defer workertest.CleanKill(c, watcher1)
	var watcher2 worker.Worker
	if allowUnits {
		c.Assert(result.Results[2].NotifyWatcherId, gc.Equals, "2")
		watcher2 = s.resources.Get("2")
		defer workertest.CleanKill(c, watcher2)
	}

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc1 := statetesting.NewNotifyWatcherC(c, watcher1.(state.NotifyWatcher))
	wc1.AssertNoChange()
	if allowUnits {
		wc2 := statetesting.NewNotifyWatcherC(c, watcher2.(state.NotifyWatcher))
		wc2.AssertNoChange()
	}
}

func (s *firewallerBaseSuite) testWatchUnits(
	c *gc.C,
	facade interface {
		WatchUnits(context.Context, params.Entities) (params.StringsWatchResults, error)
	},
) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.application.Tag().String()},
		{Tag: s.units[0].Tag().String()},
	}})
	result, err := facade.WatchUnits(context.Background(), args)
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
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *firewallerBaseSuite) testGetAssignedMachine(
	c *gc.C,
	facade interface {
		GetAssignedMachine(ctx context.Context, args params.Entities) (params.StringResults, error)
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
	result, err := facade.GetAssignedMachine(context.Background(), args)
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
	result, err = facade.GetAssignedMachine(context.Background(), args)
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
	{Tag: "application-bar"},
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
