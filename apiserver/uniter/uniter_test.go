// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	patchtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/apiserver/uniter"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	jujuFactory "github.com/juju/juju/testing/factory"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type uniterSuite struct {
	testing.JujuConnSuite
	*commontesting.EnvironWatcherTest

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources

	machine0      *state.Machine
	machine1      *state.Machine
	wordpress     *state.Service
	wpCharm       *state.Charm
	mysql         *state.Service
	wordpressUnit *state.Unit
	mysqlUnit     *state.Unit

	uniter *uniter.UniterAPI
}

var _ = gc.Suite(&uniterSuite{})

func (s *uniterSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	factory := jujuFactory.NewFactory(s.State)
	// Create two machines, two services and add a unit to each service.
	s.machine0 = factory.MakeMachine(c, &jujuFactory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits, state.JobManageEnviron},
	})
	s.machine1 = factory.MakeMachine(c, &jujuFactory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})
	s.wpCharm = factory.MakeCharm(c, &jujuFactory.CharmParams{
		Name: "wordpress",
	})
	s.wordpress = factory.MakeService(c, &jujuFactory.ServiceParams{
		Name:    "wordpress",
		Charm:   s.wpCharm,
		Creator: s.AdminUserTag(c),
	})
	mysqlCharm := factory.MakeCharm(c, &jujuFactory.CharmParams{
		Name: "mysql",
	})
	s.mysql = factory.MakeService(c, &jujuFactory.ServiceParams{
		Name:    "mysql",
		Charm:   mysqlCharm,
		Creator: s.AdminUserTag(c),
	})
	s.wordpressUnit = factory.MakeUnit(c, &jujuFactory.UnitParams{
		Service: s.wordpress,
		Machine: s.machine0,
	})
	s.mysqlUnit = factory.MakeUnit(c, &jujuFactory.UnitParams{
		Service: s.mysql,
		Machine: s.machine1,
	})

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming unit 0 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.wordpressUnit.Tag(),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	// Create a uniter API for unit 0.
	var err error
	s.uniter, err = uniter.NewUniterAPI(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, gc.IsNil)
	s.EnvironWatcherTest = commontesting.NewEnvironWatcherTest(s.uniter, s.State, s.resources, commontesting.NoSecrets)
}

func (s *uniterSuite) TestUniterFailsWithNonUnitAgentUser(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("9")
	anUniter, err := uniter.NewUniterAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.NotNil)
	c.Assert(anUniter, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *uniterSuite) TestSetStatus(c *gc.C) {
	err := s.wordpressUnit.SetStatus(state.StatusStarted, "blah", nil)
	c.Assert(err, gc.IsNil)
	err = s.mysqlUnit.SetStatus(state.StatusStopped, "foo", nil)
	c.Assert(err, gc.IsNil)

	args := params.SetStatus{
		Entities: []params.EntityStatus{
			{Tag: "unit-mysql-0", Status: params.StatusError, Info: "not really"},
			{Tag: "unit-wordpress-0", Status: params.StatusStopped, Info: "foobar"},
			{Tag: "unit-foo-42", Status: params.StatusStarted, Info: "blah"},
		}}
	result, err := s.uniter.SetStatus(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify mysqlUnit - no change.
	status, info, _, err := s.mysqlUnit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, state.StatusStopped)
	c.Assert(info, gc.Equals, "foo")
	// ...wordpressUnit is fine though.
	status, info, _, err = s.wordpressUnit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, state.StatusStopped)
	c.Assert(info, gc.Equals, "foobar")
}

func (s *uniterSuite) TestLife(c *gc.C) {
	// Add a relation wordpress-mysql.
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, gc.IsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(rel.Life(), gc.Equals, state.Alive)

	// Make the wordpressUnit dead.
	err = s.wordpressUnit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Dead)

	// Add another unit, so the service will stay dying when we
	// destroy it later.
	extraUnit, err := s.wordpress.AddUnit()
	c.Assert(err, gc.IsNil)
	c.Assert(extraUnit, gc.NotNil)

	// Make the wordpress service dying.
	err = s.wordpress.Destroy()
	c.Assert(err, gc.IsNil)
	err = s.wordpress.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.wordpress.Life(), gc.Equals, state.Dying)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "service-mysql"},
		{Tag: "service-wordpress"},
		{Tag: "service-foo"},
		// TODO(dfc) these aren't valid tags any more
		// but I hope to restore this test when params.Entity takes
		// tags, not strings, which is coming soon.
		// {Tag: "just-foo"},
		{Tag: rel.Tag().String()},
		{Tag: "relation-svc1.rel1#svc2.rel2"},
		// {Tag: "relation-blah"},
	}}
	result, err := s.uniter.Life(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "dead"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "dying"},
			{Error: apiservertesting.ErrUnauthorized},
			// TODO(dfc) see above
			// {Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			// {Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestEnsureDead(c *gc.C) {
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Alive)
	c.Assert(s.mysqlUnit.Life(), gc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.EnsureDead(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Dead)
	err = s.mysqlUnit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.mysqlUnit.Life(), gc.Equals, state.Alive)

	// Try it again on a Dead unit; should work.
	args = params.Entities{
		Entities: []params.Entity{{Tag: "unit-wordpress-0"}},
	}
	result, err = s.uniter.EnsureDead(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{nil}},
	})

	// Verify Life is unchanged.
	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Dead)
}

func (s *uniterSuite) assertOneStringsWatcher(c *gc.C, result params.StringsWatchResults, err error) {
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(result.Results[1].StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Results[1].Changes, gc.NotNil)
	c.Assert(result.Results[1].Error, gc.IsNil)
	c.Assert(result.Results[2].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatch(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "service-mysql"},
		{Tag: "service-wordpress"},
		{Tag: "service-foo"},
		// TODO(dfc) these aren't valid tags any more
		// but I hope to restore this test when params.Entity takes
		// tags, not strings, which is coming soon.
		// {Tag: "just-foo"},
	}}
	result, err := s.uniter.Watch(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{NotifyWatcherId: "2"},
			{Error: apiservertesting.ErrUnauthorized},
			// see above
			// {Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 2)
	resource1 := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource1)
	resource2 := s.resources.Get("2")
	defer statetesting.AssertStop(c, resource2)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, s.State, resource1.(state.NotifyWatcher))
	wc.AssertNoChange()
	wc = statetesting.NewNotifyWatcherC(c, s.State, resource2.(state.NotifyWatcher))
	wc.AssertNoChange()
}

func (s *uniterSuite) TestPublicAddress(c *gc.C) {
	// Try first without setting an address.
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	expectErr := &params.Error{
		Code:    params.CodeNoAddressSet,
		Message: `"unit-wordpress-0" has no public address set`,
	}
	result, err := s.uniter.PublicAddress(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: expectErr},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now set it an try again.
	err = s.machine0.SetAddresses(network.NewAddress("1.2.3.4", network.ScopePublic))
	c.Assert(err, gc.IsNil)
	address, ok := s.wordpressUnit.PublicAddress()
	c.Assert(address, gc.Equals, "1.2.3.4")
	c.Assert(ok, jc.IsTrue)

	result, err = s.uniter.PublicAddress(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "1.2.3.4"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestPrivateAddress(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	expectErr := &params.Error{
		Code:    params.CodeNoAddressSet,
		Message: `"unit-wordpress-0" has no private address set`,
	}
	result, err := s.uniter.PrivateAddress(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: expectErr},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now set it and try again.
	err = s.machine0.SetAddresses(network.NewAddress("1.2.3.4", network.ScopeCloudLocal))
	c.Assert(err, gc.IsNil)
	address, ok := s.wordpressUnit.PrivateAddress()
	c.Assert(address, gc.Equals, "1.2.3.4")
	c.Assert(ok, jc.IsTrue)

	result, err = s.uniter.PrivateAddress(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "1.2.3.4"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestResolved(c *gc.C) {
	err := s.wordpressUnit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, gc.IsNil)
	mode := s.wordpressUnit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedRetryHooks)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.Resolved(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ResolvedModeResults{
		Results: []params.ResolvedModeResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Mode: params.ResolvedMode(mode)},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestClearResolved(c *gc.C) {
	err := s.wordpressUnit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, gc.IsNil)
	mode := s.wordpressUnit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedRetryHooks)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.ClearResolved(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify wordpressUnit's resolved mode has changed.
	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	mode = s.wordpressUnit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNone)
}

func (s *uniterSuite) TestGetPrincipal(c *gc.C) {
	// Add a subordinate to wordpressUnit.
	_, _, subordinate := s.addRelatedService(c, "wordpress", "logging", s.wordpressUnit)

	principal, ok := subordinate.PrincipalName()
	c.Assert(principal, gc.Equals, s.wordpressUnit.Name())
	c.Assert(ok, jc.IsTrue)

	// First try it as wordpressUnit's agent.
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: subordinate.Tag().String()},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.GetPrincipal(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringBoolResults{
		Results: []params.StringBoolResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "", Ok: false, Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now try as subordinate's agent.
	subAuthorizer := s.authorizer
	subAuthorizer.Tag = subordinate.Tag()
	subUniter, err := uniter.NewUniterAPI(s.State, s.resources, subAuthorizer)
	c.Assert(err, gc.IsNil)

	result, err = subUniter.GetPrincipal(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringBoolResults{
		Results: []params.StringBoolResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "unit-wordpress-0", Ok: true, Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) addRelatedService(c *gc.C, firstSvc, relatedSvc string, unit *state.Unit) (*state.Relation, *state.Service, *state.Unit) {
	relatedService := s.AddTestingService(c, relatedSvc, s.AddTestingCharm(c, relatedSvc))
	rel := s.addRelation(c, firstSvc, relatedSvc)
	relUnit, err := rel.Unit(unit)
	c.Assert(err, gc.IsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	relatedUnit, err := s.State.Unit(relatedSvc + "/0")
	c.Assert(err, gc.IsNil)
	return rel, relatedService, relatedUnit
}

func (s *uniterSuite) TestHasSubordinates(c *gc.C) {
	// Try first without any subordinates for wordpressUnit.
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-logging-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.HasSubordinates(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: false},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Add two subordinates to wordpressUnit and try again.
	s.addRelatedService(c, "wordpress", "logging", s.wordpressUnit)
	s.addRelatedService(c, "wordpress", "monitoring", s.wordpressUnit)

	result, err = s.uniter.HasSubordinates(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: true},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestDestroy(c *gc.C) {
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.Destroy(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify wordpressUnit is destroyed and removed.
	err = s.wordpressUnit.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *uniterSuite) TestDestroyAllSubordinates(c *gc.C) {
	// Add two subordinates to wordpressUnit.
	_, _, loggingSub := s.addRelatedService(c, "wordpress", "logging", s.wordpressUnit)
	_, _, monitoringSub := s.addRelatedService(c, "wordpress", "monitoring", s.wordpressUnit)
	c.Assert(loggingSub.Life(), gc.Equals, state.Alive)
	c.Assert(monitoringSub.Life(), gc.Equals, state.Alive)

	err := s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	subordinates := s.wordpressUnit.SubordinateNames()
	c.Assert(subordinates, gc.DeepEquals, []string{"logging/0", "monitoring/0"})

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.DestroyAllSubordinates(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify wordpressUnit's subordinates were destroyed.
	err = loggingSub.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(loggingSub.Life(), gc.Equals, state.Dying)
	err = monitoringSub.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(monitoringSub.Life(), gc.Equals, state.Dying)
}

func (s *uniterSuite) TestCharmURL(c *gc.C) {
	// Set wordpressUnit's charm URL first.
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, gc.IsNil)
	curl, ok := s.wordpressUnit.CharmURL()
	c.Assert(curl, gc.DeepEquals, s.wpCharm.URL())
	c.Assert(ok, jc.IsTrue)

	// Make sure wordpress service's charm is what we expect.
	curl, force := s.wordpress.CharmURL()
	c.Assert(curl, gc.DeepEquals, s.wpCharm.URL())
	c.Assert(force, jc.IsFalse)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "service-mysql"},
		{Tag: "service-wordpress"},
		{Tag: "service-foo"},
		// TODO(dfc) these aren't valid tags any more
		// but I hope to restore this test when params.Entity takes
		// tags, not strings, which is coming soon.
		// {Tag: "just-foo"},
	}}
	result, err := s.uniter.CharmURL(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringBoolResults{
		Results: []params.StringBoolResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: s.wpCharm.String(), Ok: ok},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Result: s.wpCharm.String(), Ok: force},
			{Error: apiservertesting.ErrUnauthorized},
			// see above
			// {Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestSetCharmURL(c *gc.C) {
	_, ok := s.wordpressUnit.CharmURL()
	c.Assert(ok, jc.IsFalse)

	args := params.EntitiesCharmURL{Entities: []params.EntityCharmURL{
		{Tag: "unit-mysql-0", CharmURL: "cs:quantal/service-42"},
		{Tag: "unit-wordpress-0", CharmURL: s.wpCharm.String()},
		{Tag: "unit-foo-42", CharmURL: "cs:quantal/foo-321"},
	}}
	result, err := s.uniter.SetCharmURL(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the charm URL was set.
	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	charmUrl, needsUpgrade := s.wordpressUnit.CharmURL()
	c.Assert(charmUrl, gc.NotNil)
	c.Assert(charmUrl.String(), gc.Equals, s.wpCharm.String())
	c.Assert(needsUpgrade, jc.IsTrue)
}

func (s *uniterSuite) TestOpenPorts(c *gc.C) {
	openedPorts, err := s.wordpressUnit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(openedPorts, gc.HasLen, 0)

	args := params.EntitiesPortRanges{Entities: []params.EntityPortRange{
		{Tag: "unit-mysql-0", Protocol: "tcp", FromPort: 1234, ToPort: 1400},
		{Tag: "unit-wordpress-0", Protocol: "udp", FromPort: 4321, ToPort: 5000},
		{Tag: "unit-foo-42", Protocol: "tcp", FromPort: 42, ToPort: 42},
	}}
	result, err := s.uniter.OpenPorts(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the wordpressUnit's port is opened.
	openedPorts, err = s.wordpressUnit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(openedPorts, gc.DeepEquals, []network.PortRange{
		{Protocol: "udp", FromPort: 4321, ToPort: 5000},
	})
}

func (s *uniterSuite) TestClosePorts(c *gc.C) {
	// Open port udp:4321 in advance on wordpressUnit.
	err := s.wordpressUnit.OpenPorts("udp", 4321, 5000)
	c.Assert(err, gc.IsNil)
	openedPorts, err := s.wordpressUnit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(openedPorts, gc.DeepEquals, []network.PortRange{
		{Protocol: "udp", FromPort: 4321, ToPort: 5000},
	})

	args := params.EntitiesPortRanges{Entities: []params.EntityPortRange{
		{Tag: "unit-mysql-0", Protocol: "tcp", FromPort: 1234, ToPort: 1400},
		{Tag: "unit-wordpress-0", Protocol: "udp", FromPort: 4321, ToPort: 5000},
		{Tag: "unit-foo-42", Protocol: "tcp", FromPort: 42, ToPort: 42},
	}}
	result, err := s.uniter.ClosePorts(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the wordpressUnit's port is closed.
	openedPorts, err = s.wordpressUnit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(openedPorts, gc.HasLen, 0)
}

func (s *uniterSuite) TestOpenPort(c *gc.C) {
	openedPorts, err := s.wordpressUnit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(openedPorts, gc.HasLen, 0)

	args := params.EntitiesPorts{Entities: []params.EntityPort{
		{Tag: "unit-mysql-0", Protocol: "tcp", Port: 1234},
		{Tag: "unit-wordpress-0", Protocol: "udp", Port: 4321},
		{Tag: "unit-foo-42", Protocol: "tcp", Port: 42},
	}}
	result, err := s.uniter.OpenPort(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the wordpressUnit's port is opened.
	openedPorts, err = s.wordpressUnit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(openedPorts, gc.DeepEquals, []network.PortRange{
		{Protocol: "udp", FromPort: 4321, ToPort: 4321},
	})
}

func (s *uniterSuite) TestClosePort(c *gc.C) {
	// Open port udp:4321 in advance on wordpressUnit.
	err := s.wordpressUnit.OpenPort("udp", 4321)
	c.Assert(err, gc.IsNil)
	openedPorts, err := s.wordpressUnit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(openedPorts, gc.DeepEquals, []network.PortRange{
		{Protocol: "udp", FromPort: 4321, ToPort: 4321},
	})

	args := params.EntitiesPorts{Entities: []params.EntityPort{
		{Tag: "unit-mysql-0", Protocol: "tcp", Port: 1234},
		{Tag: "unit-wordpress-0", Protocol: "udp", Port: 4321},
		{Tag: "unit-foo-42", Protocol: "tcp", Port: 42},
	}}
	result, err := s.uniter.ClosePort(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the wordpressUnit's port is closed.
	openedPorts, err = s.wordpressUnit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(openedPorts, gc.HasLen, 0)
}

func (s *uniterSuite) TestWatchConfigSettings(c *gc.C) {
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, gc.IsNil)

	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.WatchConfigSettings(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, s.State, resource.(state.NotifyWatcher))
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchActions(c *gc.C) {
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, gc.IsNil)

	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.WatchActions(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{StringsWatcherId: "1", Changes: []string{}},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	addedAction, err := s.wordpressUnit.AddAction("snapshot", nil)

	wc.AssertChange(addedAction.Id())
	wc.AssertNoChange()
}

func checkUnorderedActionIdsEqual(c *gc.C, ids []string, results params.StringsWatchResults) {
	c.Assert(results, gc.NotNil)
	content := results.Results
	c.Assert(len(content), gc.Equals, 1)
	result := content[0]
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	obtainedIds := map[string]int{}
	expectedIds := map[string]int{}
	for _, id := range ids {
		expectedIds[id]++
	}
	// The count of each ID that has been seen.
	for _, change := range result.Changes {
		obtainedIds[change]++
	}
	c.Check(obtainedIds, jc.DeepEquals, expectedIds)
}

func (s *uniterSuite) TestWatchPreexistingActions(c *gc.C) {
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, gc.IsNil)

	c.Assert(s.resources.Count(), gc.Equals, 0)

	action1, err := s.wordpressUnit.AddAction("backup", nil)
	c.Assert(err, gc.IsNil)
	action2, err := s.wordpressUnit.AddAction("snapshot", nil)
	c.Assert(err, gc.IsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-wordpress-0"},
	}}

	results, err := s.uniter.WatchActions(args)
	c.Assert(err, gc.IsNil)

	checkUnorderedActionIdsEqual(c, []string{action1.Id(), action2.Id()}, results)

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	addedAction, err := s.wordpressUnit.AddAction("backup", nil)
	c.Assert(err, gc.IsNil)
	wc.AssertChange(addedAction.Id())
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchActionsMalformedTag(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "ewenit-mysql-0"},
	}}
	_, err := s.uniter.WatchActions(args)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `"ewenit-mysql-0" is not a valid tag`)
}

func (s *uniterSuite) TestWatchActionsMalformedUnitName(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-01"},
	}}
	_, err := s.uniter.WatchActions(args)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `"unit-mysql-01" is not a valid unit tag`)
}

func (s *uniterSuite) TestWatchActionsNotUnit(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "action-mysql/0_a_0"},
	}}
	_, err := s.uniter.WatchActions(args)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `"action-mysql/0_a_0" is not a valid unit tag`)
}

func (s *uniterSuite) TestWatchActionsPermissionDenied(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-nonexistentgarbage-0"},
	}}
	results, err := s.uniter.WatchActions(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.NotNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.NotNil)
	c.Assert(result.Error.Message, gc.Equals, "permission denied")
}

func (s *uniterSuite) TestConfigSettings(c *gc.C) {
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, gc.IsNil)
	settings, err := s.wordpressUnit.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.ConfigSettings(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ConfigSettingsResults{
		Results: []params.ConfigSettingsResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Settings: params.ConfigSettings{"blog-title": "My Title"}},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestWatchServiceRelations(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "service-mysql"},
		{Tag: "service-wordpress"},
		{Tag: "service-foo"},
	}}
	result, err := s.uniter.WatchServiceRelations(args)
	s.assertOneStringsWatcher(c, result, err)
}

func (s *uniterSuite) TestCharmArchiveSha256(c *gc.C) {
	dummyCharm := s.AddTestingCharm(c, "dummy")

	args := params.CharmURLs{URLs: []params.CharmURL{
		{URL: "something-invalid"},
		{URL: s.wpCharm.String()},
		{URL: dummyCharm.String()},
	}}
	result, err := s.uniter.CharmArchiveSha256(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: s.wpCharm.BundleSha256()},
			{Result: dummyCharm.BundleSha256()},
		},
	})
}

func (s *uniterSuite) TestCurrentEnvironUUID(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)

	result, err := s.uniter.CurrentEnvironUUID()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResult{Result: env.UUID()})
}

func (s *uniterSuite) TestCurrentEnvironment(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)

	result, err := s.uniter.CurrentEnvironment()
	c.Assert(err, gc.IsNil)
	expected := params.EnvironmentResult{
		Name: env.Name(),
		UUID: env.UUID(),
	}
	c.Assert(result, gc.DeepEquals, expected)
}

func (s *uniterSuite) TestAction(c *gc.C) {
	var actionTests = []struct {
		description string
		action      params.ActionItem
	}{{
		description: "A simple action.",
		action: params.ActionItem{
			Name: "snapshot",
			Parameters: map[string]interface{}{
				"outfile": "foo.txt",
			},
		},
	}, {
		description: "An action with nested parameters.",
		action: params.ActionItem{
			Name: "backup",
			Parameters: map[string]interface{}{
				"outfile": "foo.bz2",
				"compression": map[string]interface{}{
					"kind":    "bzip",
					"quality": 5,
				},
			},
		},
	}}

	for i, actionTest := range actionTests {
		c.Logf("test %d: %s", i, actionTest.description)

		a, err := s.wordpressUnit.AddAction(
			actionTest.action.Name,
			actionTest.action.Parameters)
		c.Assert(err, gc.IsNil)
		actionTag := names.JoinActionTag(s.wordpressUnit.UnitTag().Id(), i)
		c.Assert(a.ActionTag(), gc.Equals, actionTag)

		args := params.Entities{
			Entities: []params.Entity{{
				Tag: actionTag.String(),
			}},
		}
		results, err := s.uniter.Actions(args)
		c.Assert(err, gc.IsNil)
		c.Assert(results.Results, gc.HasLen, 1)

		actionsQueryResult := results.Results[0]

		c.Assert(actionsQueryResult.Error, gc.IsNil)
		c.Assert(actionsQueryResult.Action, jc.DeepEquals, actionTest.action)
	}
}

func (s *uniterSuite) TestActionNotPresent(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: names.JoinActionTag("wordpress/0", 0).String(),
		}},
	}
	results, err := s.uniter.Actions(args)
	c.Assert(err, gc.IsNil)

	c.Assert(results.Results, gc.HasLen, 1)
	actionsQueryResult := results.Results[0]
	c.Assert(actionsQueryResult.Error, gc.NotNil)
	c.Assert(actionsQueryResult.Error, gc.ErrorMatches, `action .*wordpress/0[^0-9]+0[^0-9]+ not found`)
}

func (s *uniterSuite) TestActionWrongUnit(c *gc.C) {
	// Action doesn't match unit.
	fakeBadAuth := apiservertesting.FakeAuthorizer{
		Tag: s.mysqlUnit.Tag(),
	}
	fakeBadAPI, err := uniter.NewUniterAPI(
		s.State,
		s.resources,
		fakeBadAuth,
	)
	c.Assert(err, gc.IsNil)

	restore := patchtesting.PatchValue(&s.uniter, fakeBadAPI)
	defer restore()

	args := params.Entities{
		Entities: []params.Entity{{
			Tag: names.JoinActionTag("wordpress/0", 0).String(),
		}},
	}
	actions, err := s.uniter.Actions(args)
	c.Assert(err, gc.IsNil)
	c.Assert(len(actions.Results), gc.Equals, 1)
	isErrPerm(c, actions.Results[0].Error)
}

func (s *uniterSuite) TestActionPermissionDenied(c *gc.C) {
	// Same unit, but not one that has access.
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: names.JoinActionTag("mysql/0", 0).String(),
		}},
	}
	actions, err := s.uniter.Actions(args)
	c.Assert(err, gc.IsNil)
	c.Assert(len(actions.Results), gc.Equals, 1)
	isErrPerm(c, actions.Results[0].Error)
}

func isErrPerm(c *gc.C, err *params.Error) {
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, common.ErrPerm.Error())
}

func (s *uniterSuite) TestActionComplete(c *gc.C) {
	testName := "frobz"
	testOutput := map[string]interface{}{"output": "completed frobz successfully"}

	results, err := s.wordpressUnit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, ([]*state.ActionResult)(nil))

	action, err := s.wordpressUnit.AddAction(testName, nil)
	c.Assert(err, gc.IsNil)

	actionResults := params.ActionExecutionResults{
		Results: []params.ActionExecutionResult{{
			ActionTag: action.ActionTag().String(),
			Status:    params.ActionCompleted,
			Results:   testOutput,
		}},
	}

	res, err := s.uniter.FinishActions(actionResults)
	c.Assert(err, gc.IsNil)
	c.Assert(res, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}})

	results, err = s.wordpressUnit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(len(results), gc.Equals, 1)
	c.Assert(results[0].Status(), gc.Equals, state.ActionCompleted)
	res2, errstr := results[0].Results()
	c.Assert(errstr, gc.Equals, "")
	c.Assert(res2, gc.DeepEquals, testOutput)
	c.Assert(results[0].ActionName(), gc.Equals, testName)
}

func (s *uniterSuite) TestActionFail(c *gc.C) {
	testName := "wgork"
	testError := "wgork was a dismal failure"

	results, err := s.wordpressUnit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, ([]*state.ActionResult)(nil))

	action, err := s.wordpressUnit.AddAction(testName, nil)
	c.Assert(err, gc.IsNil)

	actionResults := params.ActionExecutionResults{
		Results: []params.ActionExecutionResult{{
			ActionTag: action.ActionTag().String(),
			Status:    params.ActionFailed,
			Results:   nil,
			Message:   testError,
		}},
	}

	res, err := s.uniter.FinishActions(actionResults)
	c.Assert(err, gc.IsNil)
	c.Assert(res, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}})

	results, err = s.wordpressUnit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(len(results), gc.Equals, 1)
	c.Assert(results[0].Status(), gc.Equals, state.ActionFailed)
	res2, errstr := results[0].Results()
	c.Assert(errstr, gc.Equals, testError)
	c.Assert(res2, gc.DeepEquals, map[string]interface{}{})
	c.Assert(results[0].ActionName(), gc.Equals, testName)
}

func (s *uniterSuite) TestFinishActionAuthAccess(c *gc.C) {

	good, err := s.wordpressUnit.AddAction("fakeaction", nil)
	c.Assert(err, gc.IsNil)

	bad, err := s.mysqlUnit.AddAction("fakeaction", nil)
	c.Assert(err, gc.IsNil)

	var tests = []struct {
		actionTag string
		err       error
	}{
		{actionTag: "", err: errors.Errorf(`"" is not a valid tag`)},
		{actionTag: s.machine0.Tag().String(), err: errors.Errorf(`"machine-0" is not a valid action tag`)},
		{actionTag: s.mysql.Tag().String(), err: errors.Errorf(`"service-mysql" is not a valid action tag`)},
		{actionTag: good.Tag().String(), err: nil},
		{actionTag: bad.Tag().String(), err: common.ErrPerm},
		{actionTag: "asdf", err: errors.Errorf(`"asdf" is not a valid tag`)},
	}

	// Queue up actions from tests
	actionResults := params.ActionExecutionResults{Results: make([]params.ActionExecutionResult, len(tests))}
	for i, test := range tests {
		actionResults.Results[i] = params.ActionExecutionResult{
			ActionTag: test.actionTag,
			Status:    params.ActionCompleted,
			Results:   map[string]interface{}{},
		}
	}

	// Invoke FinishActions
	res, err := s.uniter.FinishActions(actionResults)
	c.Assert(err, gc.IsNil)

	// Verify permissions errors for actions queued on different unit
	for i, result := range res.Results {
		expected := tests[i].err
		if expected != nil {
			c.Assert(result.Error, gc.NotNil)
			c.Assert(result.Error.Error(), gc.Equals, expected.Error())
		} else {
			c.Assert(result.Error, gc.IsNil)
		}
	}
}

func (s *uniterSuite) addRelation(c *gc.C, first, second string) *state.Relation {
	eps, err := s.State.InferEndpoints(first, second)
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	return rel
}

func (s *uniterSuite) TestRelation(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	wpEp, err := rel.Endpoint("wordpress")
	c.Assert(err, gc.IsNil)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-42", Unit: "unit-foo-0"},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0"},
		{Relation: rel.Tag().String(), Unit: "unit-foo-0"},
		{Relation: "relation-blah", Unit: "unit-wordpress-0"},
		{Relation: "service-foo", Unit: "user-foo"},
		{Relation: "foo", Unit: "bar"},
		{Relation: "unit-wordpress-0", Unit: rel.Tag().String()},
	}}
	result, err := s.uniter.Relation(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.RelationResults{
		Results: []params.RelationResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				Id:   rel.Id(),
				Key:  rel.String(),
				Life: params.Life(rel.Life().String()),
				Endpoint: params.Endpoint{
					ServiceName: wpEp.ServiceName,
					Relation:    wpEp.Relation,
				},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestRelationById(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	c.Assert(rel.Id(), gc.Equals, 0)
	wpEp, err := rel.Endpoint("wordpress")
	c.Assert(err, gc.IsNil)

	// Add another relation to mysql service, so we can see we can't
	// get it.
	otherRel, _, _ := s.addRelatedService(c, "mysql", "logging", s.mysqlUnit)

	args := params.RelationIds{
		RelationIds: []int{-1, rel.Id(), otherRel.Id(), 42, 234},
	}
	result, err := s.uniter.RelationById(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.RelationResults{
		Results: []params.RelationResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				Id:   rel.Id(),
				Key:  rel.String(),
				Life: params.Life(rel.Life().String()),
				Endpoint: params.Endpoint{
					ServiceName: wpEp.ServiceName,
					Relation:    wpEp.Relation,
				},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestProviderType(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)

	result, err := s.uniter.ProviderType()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResult{Result: cfg.Type()})
}

func (s *uniterSuite) assertInScope(c *gc.C, relUnit *state.RelationUnit, inScope bool) {
	ok, err := relUnit.InScope()
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, inScope)
}

func (s *uniterSuite) TestEnterScope(c *gc.C) {
	// Set wordpressUnit's private address first.
	err := s.machine0.SetAddresses(network.NewAddress("1.2.3.4", network.ScopeCloudLocal))
	c.Assert(err, gc.IsNil)

	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, relUnit, false)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-42", Unit: "unit-foo-0"},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
		{Relation: "relation-42", Unit: "unit-wordpress-0"},
		{Relation: "relation-foo", Unit: "unit-wordpress-0"},
		{Relation: "service-wordpress", Unit: "unit-foo-0"},
		{Relation: "foo", Unit: "bar"},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0"},
		{Relation: rel.Tag().String(), Unit: "service-wordpress"},
		{Relation: rel.Tag().String(), Unit: "service-mysql"},
		{Relation: rel.Tag().String(), Unit: "user-foo"},
	}}
	result, err := s.uniter.EnterScope(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the scope changes and settings.
	s.assertInScope(c, relUnit, true)
	readSettings, err := relUnit.ReadSettings(s.wordpressUnit.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(readSettings, gc.DeepEquals, map[string]interface{}{
		"private-address": "1.2.3.4",
	})
}

func (s *uniterSuite) TestLeaveScope(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, gc.IsNil)
	settings := map[string]interface{}{
		"some": "settings",
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, relUnit, true)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-42", Unit: "unit-foo-0"},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
		{Relation: "relation-42", Unit: "unit-wordpress-0"},
		{Relation: "relation-foo", Unit: "unit-wordpress-0"},
		{Relation: "service-wordpress", Unit: "unit-foo-0"},
		{Relation: "foo", Unit: "bar"},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0"},
		{Relation: rel.Tag().String(), Unit: "service-wordpress"},
		{Relation: rel.Tag().String(), Unit: "service-mysql"},
		{Relation: rel.Tag().String(), Unit: "user-foo"},
	}}
	result, err := s.uniter.LeaveScope(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the scope changes.
	s.assertInScope(c, relUnit, false)
	readSettings, err := relUnit.ReadSettings(s.wordpressUnit.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(readSettings, gc.DeepEquals, settings)
}

func (s *uniterSuite) TestJoinedRelations(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, gc.IsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)

	args := params.Entities{
		Entities: []params.Entity{
			{s.wordpressUnit.Tag().String()},
			{s.mysqlUnit.Tag().String()},
			{"unit-unknown-1"},
			{"service-wordpress"},
			{"machine-0"},
			{rel.Tag().String()},
		},
	}
	expect := params.StringsResults{
		Results: []params.StringsResult{
			{Result: []string{rel.Tag().String()}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	}
	check := func() {
		result, err := s.uniter.JoinedRelations(args)
		c.Assert(err, gc.IsNil)
		c.Assert(result, gc.DeepEquals, expect)
	}
	check()
	err = relUnit.PrepareLeaveScope()
	c.Assert(err, gc.IsNil)
	check()
}

func (s *uniterSuite) TestReadSettings(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, gc.IsNil)
	settings := map[string]interface{}{
		"some": "settings",
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, relUnit, true)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-42", Unit: "unit-foo-0"},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0"},
		{Relation: "relation-42", Unit: "unit-wordpress-0"},
		{Relation: "relation-foo", Unit: ""},
		{Relation: "service-wordpress", Unit: "unit-foo-0"},
		{Relation: "foo", Unit: "bar"},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0"},
		{Relation: rel.Tag().String(), Unit: "service-wordpress"},
		{Relation: rel.Tag().String(), Unit: "service-mysql"},
		{Relation: rel.Tag().String(), Unit: "user-foo"},
	}}
	result, err := s.uniter.ReadSettings(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.RelationSettingsResults{
		Results: []params.RelationSettingsResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Settings: params.RelationSettings{
				"some": "settings",
			}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestReadSettingsWithNonStringValuesFails(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, gc.IsNil)
	settings := map[string]interface{}{
		"other":        "things",
		"invalid-bool": false,
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, relUnit, true)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
	}}
	expectErr := `unexpected relation setting "invalid-bool": expected string, got bool`
	result, err := s.uniter.ReadSettings(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.RelationSettingsResults{
		Results: []params.RelationSettingsResult{
			{Error: &params.Error{Message: expectErr}},
		},
	})
}

func (s *uniterSuite) TestReadRemoteSettings(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, gc.IsNil)
	settings := map[string]interface{}{
		"some": "settings",
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, relUnit, true)

	// First test most of the invalid args tests and try to read the
	// (unset) remote unit settings.
	args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{
		{Relation: "relation-42", LocalUnit: "unit-foo-0", RemoteUnit: "foo"},
		{Relation: rel.Tag().String(), LocalUnit: "unit-wordpress-0", RemoteUnit: "unit-wordpress-0"},
		{Relation: rel.Tag().String(), LocalUnit: "unit-wordpress-0", RemoteUnit: "unit-mysql-0"},
		{Relation: "relation-42", LocalUnit: "unit-wordpress-0", RemoteUnit: ""},
		{Relation: "relation-foo", LocalUnit: "", RemoteUnit: ""},
		{Relation: "service-wordpress", LocalUnit: "unit-foo-0", RemoteUnit: "user-foo"},
		{Relation: "foo", LocalUnit: "bar", RemoteUnit: "baz"},
		{Relation: rel.Tag().String(), LocalUnit: "unit-mysql-0", RemoteUnit: "unit-wordpress-0"},
		{Relation: rel.Tag().String(), LocalUnit: "service-wordpress", RemoteUnit: "service-mysql"},
		{Relation: rel.Tag().String(), LocalUnit: "service-mysql", RemoteUnit: "foo"},
		{Relation: rel.Tag().String(), LocalUnit: "user-foo", RemoteUnit: "unit-wordpress-0"},
	}}
	result, err := s.uniter.ReadRemoteSettings(args)

	// We don't set the remote unit settings on purpose to test the error.
	expectErr := `cannot read settings for unit "mysql/0" in relation "wordpress:db mysql:server": settings not found`
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.RelationSettingsResults{
		Results: []params.RelationSettingsResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ServerError(expectErr)},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now leave the mysqlUnit and re-enter with new settings.
	relUnit, err = rel.Unit(s.mysqlUnit)
	c.Assert(err, gc.IsNil)
	settings = map[string]interface{}{
		"other": "things",
	}
	err = relUnit.LeaveScope()
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, relUnit, false)
	err = relUnit.EnterScope(settings)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, relUnit, true)

	// Test the remote unit settings can be read.
	args = params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{{
		Relation:   rel.Tag().String(),
		LocalUnit:  "unit-wordpress-0",
		RemoteUnit: "unit-mysql-0",
	}}}
	expect := params.RelationSettingsResults{
		Results: []params.RelationSettingsResult{
			{Settings: params.RelationSettings{
				"other": "things",
			}},
		},
	}
	result, err = s.uniter.ReadRemoteSettings(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, expect)

	// Now destroy the remote unit, and check its settings can still be read.
	err = s.mysqlUnit.Destroy()
	c.Assert(err, gc.IsNil)
	err = s.mysqlUnit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.mysqlUnit.Remove()
	c.Assert(err, gc.IsNil)
	result, err = s.uniter.ReadRemoteSettings(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, expect)
}

func (s *uniterSuite) TestReadRemoteSettingsWithNonStringValuesFails(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.mysqlUnit)
	c.Assert(err, gc.IsNil)
	settings := map[string]interface{}{
		"other":        "things",
		"invalid-bool": false,
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, relUnit, true)

	args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{{
		Relation:   rel.Tag().String(),
		LocalUnit:  "unit-wordpress-0",
		RemoteUnit: "unit-mysql-0",
	}}}
	expectErr := `unexpected relation setting "invalid-bool": expected string, got bool`
	result, err := s.uniter.ReadRemoteSettings(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.RelationSettingsResults{
		Results: []params.RelationSettingsResult{
			{Error: &params.Error{Message: expectErr}},
		},
	})
}

func (s *uniterSuite) TestUpdateSettings(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, gc.IsNil)
	settings := map[string]interface{}{
		"some":  "settings",
		"other": "stuff",
	}
	err = relUnit.EnterScope(settings)
	s.assertInScope(c, relUnit, true)

	newSettings := params.RelationSettings{
		"some":  "different",
		"other": "",
	}

	args := params.RelationUnitsSettings{RelationUnits: []params.RelationUnitSettings{
		{Relation: "relation-42", Unit: "unit-foo-0", Settings: nil},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0", Settings: newSettings},
		{Relation: "relation-42", Unit: "unit-wordpress-0", Settings: nil},
		{Relation: "relation-foo", Unit: "unit-wordpress-0", Settings: nil},
		{Relation: "service-wordpress", Unit: "unit-foo-0", Settings: nil},
		{Relation: "foo", Unit: "bar", Settings: nil},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0", Settings: nil},
		{Relation: rel.Tag().String(), Unit: "service-wordpress", Settings: nil},
		{Relation: rel.Tag().String(), Unit: "service-mysql", Settings: nil},
		{Relation: rel.Tag().String(), Unit: "user-foo", Settings: nil},
	}}
	result, err := s.uniter.UpdateSettings(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the settings were saved.
	s.assertInScope(c, relUnit, true)
	readSettings, err := relUnit.ReadSettings(s.wordpressUnit.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(readSettings, gc.DeepEquals, map[string]interface{}{
		"some": "different",
	})
}

func (s *uniterSuite) TestWatchRelationUnits(c *gc.C) {
	// Add a relation between wordpress and mysql and enter scope with
	// mysqlUnit.
	rel := s.addRelation(c, "wordpress", "mysql")
	myRelUnit, err := rel.Unit(s.mysqlUnit)
	c.Assert(err, gc.IsNil)
	err = myRelUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, myRelUnit, true)

	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-42", Unit: "unit-foo-0"},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0"},
		{Relation: "relation-42", Unit: "unit-wordpress-0"},
		{Relation: "relation-foo", Unit: ""},
		{Relation: "service-wordpress", Unit: "unit-foo-0"},
		{Relation: "foo", Unit: "bar"},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0"},
		{Relation: rel.Tag().String(), Unit: "service-wordpress"},
		{Relation: rel.Tag().String(), Unit: "service-mysql"},
		{Relation: rel.Tag().String(), Unit: "user-foo"},
	}}
	result, err := s.uniter.WatchRelationUnits(args)
	c.Assert(err, gc.IsNil)
	// UnitSettings versions are volatile, so we don't check them.
	// We just make sure the keys of the Changed field are as
	// expected.
	c.Assert(result.Results, gc.HasLen, len(args.RelationUnits))
	mysqlChanges := result.Results[1].Changes
	c.Assert(mysqlChanges, gc.NotNil)
	changed, ok := mysqlChanges.Changed["mysql/0"]
	c.Assert(ok, jc.IsTrue)
	expectChanges := params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{"mysql/0": changed},
	}
	c.Assert(result, gc.DeepEquals, params.RelationUnitsWatchResults{
		Results: []params.RelationUnitsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				RelationUnitsWatcherId: "1",
				Changes:                expectChanges,
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewRelationUnitsWatcherC(c, s.State, resource.(state.RelationUnitsWatcher))
	wc.AssertNoChange()

	// Leave scope with mysqlUnit and check it's detected.
	err = myRelUnit.LeaveScope()
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, myRelUnit, false)

	wc.AssertChange(nil, []string{"mysql/0"})
}

func (s *uniterSuite) TestAPIAddresses(c *gc.C) {
	hostPorts := [][]network.HostPort{{{
		Address: network.NewAddress("0.1.2.3", network.ScopeUnknown),
		Port:    1234,
	}}}

	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, gc.IsNil)

	result, err := s.uniter.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResult{
		Result: []string{"0.1.2.3:1234"},
	})
}

func (s *uniterSuite) TestGetOwnerTag(c *gc.C) {
	tag := s.mysql.Tag().String()
	args := params.Entities{Entities: []params.Entity{
		{Tag: tag},
	}}
	result, err := s.uniter.GetOwnerTag(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResult{
		Result: s.AdminUserTag(c).String(),
	})
}

func (s *uniterSuite) TestWatchUnitAddresses(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-0"},
		{Tag: "service-wordpress"},
	}}
	result, err := s.uniter.WatchUnitAddresses(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, s.State, resource.(state.NotifyWatcher))
	wc.AssertNoChange()
}

func (s *uniterSuite) TestAddMetrics(c *gc.C) {
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, gc.IsNil)

	now := time.Now()
	sentMetrics := []params.Metric{{"A", "5", now}, {"B", "0.71", now}}
	args := params.MetricsParams{
		Metrics: []params.MetricsParam{{
			Tag:     s.wordpressUnit.Tag().String(),
			Metrics: sentMetrics,
		}},
	}
	result, err := s.uniter.AddMetrics(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	metrics, err := s.State.MetricBatches()
	c.Assert(err, gc.IsNil)
	c.Assert(metrics, gc.HasLen, 1)

	unitMetrics := metrics[0].Metrics()
	c.Assert(unitMetrics, gc.HasLen, 2)

	for i, unitMetric := range unitMetrics {
		c.Assert(unitMetric.Key, gc.Equals, sentMetrics[i].Key)
		c.Assert(unitMetric.Value, gc.Equals, sentMetrics[i].Value)
	}
}

func (s *uniterSuite) TestAddMetricsIncorrectTag(c *gc.C) {
	now := time.Now()

	tags := []string{"user-admin", "unit-nosuchunit", "thisisnotatag", "machine-0", "environment-blah"}

	for _, tag := range tags {

		args := params.MetricsParams{
			Metrics: []params.MetricsParam{{
				Tag:     tag,
				Metrics: []params.Metric{{"A", "5", now}, {"B", "0.71", now}},
			}},
		}

		result, err := s.uniter.AddMetrics(args)
		c.Assert(err, gc.IsNil)
		c.Assert(result.Results, gc.HasLen, 1)
		c.Assert(result.Results[0].Error, gc.ErrorMatches, "permission denied")
		metrics, err := s.State.MetricBatches()
		c.Assert(err, gc.IsNil)
		c.Assert(metrics, gc.HasLen, 0)
	}
}

func (s *uniterSuite) TestAddMetricsUnauthenticated(c *gc.C) {
	now := time.Now()
	args := params.MetricsParams{
		Metrics: []params.MetricsParam{{
			Tag:     s.mysqlUnit.Tag().String(),
			Metrics: []params.Metric{{"A", "5", now}, {"B", "0.71", now}},
		}},
	}
	result, err := s.uniter.AddMetrics(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "permission denied")
	metrics, err := s.State.MetricBatches()
	c.Assert(err, gc.IsNil)
	c.Assert(metrics, gc.HasLen, 0)
}
