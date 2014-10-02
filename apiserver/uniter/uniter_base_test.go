// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	jujuFactory "github.com/juju/juju/testing/factory"
)

// uniterBaseSuite implements common testing suite for all API
// versions. It's not intended to be used directly or registered as a
// suite, but embedded.
type uniterBaseSuite struct {
	testing.JujuConnSuite

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources

	machine0      *state.Machine
	machine1      *state.Machine
	wordpress     *state.Service
	wpCharm       *state.Charm
	mysql         *state.Service
	wordpressUnit *state.Unit
	mysqlUnit     *state.Unit
}

func (s *uniterBaseSuite) setUpTest(c *gc.C) {
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
}

func (s *uniterBaseSuite) testUniterFailsWithNonUnitAgentUser(
	c *gc.C,
	factory func(_ *state.State, _ *common.Resources, _ common.Authorizer) error,
) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("9")
	err := factory(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *uniterBaseSuite) testSetStatus(
	c *gc.C,
	facade interface {
		SetStatus(args params.SetStatus) (params.ErrorResults, error)
	},
) {
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
	result, err := facade.SetStatus(args)
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

func (s *uniterBaseSuite) testLife(
	c *gc.C,
	facade interface {
		Life(args params.Entities) (params.LifeResults, error)
	},
) {
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
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "machine-42"},
		{Tag: "service-foo"},
		// TODO(dfc) these aren't valid tags any more
		// but I hope to restore this test when params.Entity takes
		// tags, not strings, which is coming soon.
		// {Tag: "just-foo"},
		{Tag: rel.Tag().String()},
		{Tag: "relation-svc1.rel1#svc2.rel2"},
		// {Tag: "relation-blah"},
	}}
	result, err := facade.Life(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "dead"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "dying"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			// TODO(dfc) see above
			// {Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			// {Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterBaseSuite) testEnsureDead(
	c *gc.C,
	facade interface {
		EnsureDead(args params.Entities) (params.ErrorResults, error)
	},
) {
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Alive)
	c.Assert(s.mysqlUnit.Life(), gc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := facade.EnsureDead(args)
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
	result, err = facade.EnsureDead(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{nil}},
	})

	// Verify Life is unchanged.
	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Dead)
}

func (s *uniterBaseSuite) testWatch(
	c *gc.C,
	facade interface {
		Watch(args params.Entities) (params.NotifyWatchResults, error)
	},
) {
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
	result, err := facade.Watch(args)
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

func (s *uniterBaseSuite) testPublicAddress(
	c *gc.C,
	facade interface {
		PublicAddress(args params.Entities) (params.StringResults, error)
	},
) {
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
	result, err := facade.PublicAddress(args)
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

	result, err = facade.PublicAddress(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "1.2.3.4"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterBaseSuite) testPrivateAddress(
	c *gc.C,
	facade interface {
		PrivateAddress(args params.Entities) (params.StringResults, error)
	},
) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	expectErr := &params.Error{
		Code:    params.CodeNoAddressSet,
		Message: `"unit-wordpress-0" has no private address set`,
	}
	result, err := facade.PrivateAddress(args)
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

	result, err = facade.PrivateAddress(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "1.2.3.4"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterBaseSuite) testResolved(
	c *gc.C,
	facade interface {
		Resolved(args params.Entities) (params.ResolvedModeResults, error)
	},
) {
	err := s.wordpressUnit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, gc.IsNil)
	mode := s.wordpressUnit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedRetryHooks)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := facade.Resolved(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ResolvedModeResults{
		Results: []params.ResolvedModeResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Mode: params.ResolvedMode(mode)},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterBaseSuite) testClearResolved(
	c *gc.C,
	facade interface {
		ClearResolved(args params.Entities) (params.ErrorResults, error)
	},
) {
	err := s.wordpressUnit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, gc.IsNil)
	mode := s.wordpressUnit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedRetryHooks)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := facade.ClearResolved(args)
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

type getPrincipal interface {
	GetPrincipal(args params.Entities) (params.StringBoolResults, error)
}

func (s *uniterBaseSuite) testGetPrincipal(
	c *gc.C,
	facade getPrincipal,
	factory func(_ *state.State, _ *common.Resources, _ common.Authorizer) (getPrincipal, error),
) {
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
	result, err := facade.GetPrincipal(args)
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
	subUniter, err := factory(s.State, s.resources, subAuthorizer)
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

func (s *uniterBaseSuite) testHasSubordinates(
	c *gc.C,
	facade interface {
		HasSubordinates(args params.Entities) (params.BoolResults, error)
	},
) {
	// Try first without any subordinates for wordpressUnit.
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-logging-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := facade.HasSubordinates(args)
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

	result, err = facade.HasSubordinates(args)
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

func (s *uniterBaseSuite) testDestroy(
	c *gc.C,
	facade interface {
		Destroy(args params.Entities) (params.ErrorResults, error)
	},
) {
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := facade.Destroy(args)
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

func (s *uniterBaseSuite) testDestroyAllSubordinates(
	c *gc.C,
	facade interface {
		DestroyAllSubordinates(args params.Entities) (params.ErrorResults, error)
	},
) {
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
	result, err := facade.DestroyAllSubordinates(args)
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

func (s *uniterBaseSuite) testCharmURL(
	c *gc.C,
	facade interface {
		CharmURL(args params.Entities) (params.StringBoolResults, error)
	},
) {
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
	result, err := facade.CharmURL(args)
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

func (s *uniterBaseSuite) testSetCharmURL(
	c *gc.C,
	facade interface {
		SetCharmURL(args params.EntitiesCharmURL) (params.ErrorResults, error)
	},
) {
	_, ok := s.wordpressUnit.CharmURL()
	c.Assert(ok, jc.IsFalse)

	args := params.EntitiesCharmURL{Entities: []params.EntityCharmURL{
		{Tag: "unit-mysql-0", CharmURL: "cs:quantal/service-42"},
		{Tag: "unit-wordpress-0", CharmURL: s.wpCharm.String()},
		{Tag: "unit-foo-42", CharmURL: "cs:quantal/foo-321"},
	}}
	result, err := facade.SetCharmURL(args)
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

func (s *uniterBaseSuite) testOpenPorts(
	c *gc.C,
	facade interface {
		OpenPorts(args params.EntitiesPortRanges) (params.ErrorResults, error)
	},
) {
	openedPorts, err := s.wordpressUnit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(openedPorts, gc.HasLen, 0)

	args := params.EntitiesPortRanges{Entities: []params.EntityPortRange{
		{Tag: "unit-mysql-0", Protocol: "tcp", FromPort: 1234, ToPort: 1400},
		{Tag: "unit-wordpress-0", Protocol: "udp", FromPort: 4321, ToPort: 5000},
		{Tag: "unit-foo-42", Protocol: "tcp", FromPort: 42, ToPort: 42},
	}}
	result, err := facade.OpenPorts(args)
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

func (s *uniterBaseSuite) testClosePorts(
	c *gc.C,
	facade interface {
		ClosePorts(args params.EntitiesPortRanges) (params.ErrorResults, error)
	},
) {
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
	result, err := facade.ClosePorts(args)
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

func (s *uniterBaseSuite) testOpenPort(
	c *gc.C,
	facade interface {
		OpenPort(args params.EntitiesPorts) (params.ErrorResults, error)
	},
) {
	openedPorts, err := s.wordpressUnit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(openedPorts, gc.HasLen, 0)

	args := params.EntitiesPorts{Entities: []params.EntityPort{
		{Tag: "unit-mysql-0", Protocol: "tcp", Port: 1234},
		{Tag: "unit-wordpress-0", Protocol: "udp", Port: 4321},
		{Tag: "unit-foo-42", Protocol: "tcp", Port: 42},
	}}
	result, err := facade.OpenPort(args)
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

func (s *uniterBaseSuite) testClosePort(
	c *gc.C,
	facade interface {
		ClosePort(args params.EntitiesPorts) (params.ErrorResults, error)
	},
) {
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
	result, err := facade.ClosePort(args)
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

func (s *uniterBaseSuite) testWatchConfigSettings(
	c *gc.C,
	facade interface {
		WatchConfigSettings(args params.Entities) (params.NotifyWatchResults, error)
	},
) {
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, gc.IsNil)

	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := facade.WatchConfigSettings(args)
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

type watchActions interface {
	WatchActions(args params.Entities) (params.StringsWatchResults, error)
}

func (s *uniterBaseSuite) testWatchActions(c *gc.C, facade watchActions) {
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, gc.IsNil)

	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := facade.WatchActions(args)
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

func (s *uniterBaseSuite) testWatchPreexistingActions(c *gc.C, facade watchActions) {
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

	results, err := facade.WatchActions(args)
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

func (s *uniterBaseSuite) testWatchActionsMalformedTag(c *gc.C, facade watchActions) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "ewenit-mysql-0"},
	}}
	_, err := facade.WatchActions(args)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `"ewenit-mysql-0" is not a valid tag`)
}

func (s *uniterBaseSuite) testWatchActionsMalformedUnitName(c *gc.C, facade watchActions) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-01"},
	}}
	_, err := facade.WatchActions(args)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `"unit-mysql-01" is not a valid unit tag`)
}

func (s *uniterBaseSuite) testWatchActionsNotUnit(c *gc.C, facade watchActions) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "action-mysql/0_a_0"},
	}}
	_, err := facade.WatchActions(args)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `"action-mysql/0_a_0" is not a valid unit tag`)
}

func (s *uniterBaseSuite) testWatchActionsPermissionDenied(c *gc.C, facade watchActions) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-nonexistentgarbage-0"},
	}}
	results, err := facade.WatchActions(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.NotNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.NotNil)
	c.Assert(result.Error.Message, gc.Equals, "permission denied")
}

func (s *uniterBaseSuite) testConfigSettings(
	c *gc.C,
	facade interface {
		ConfigSettings(args params.Entities) (params.ConfigSettingsResults, error)
	},
) {
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
	result, err := facade.ConfigSettings(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ConfigSettingsResults{
		Results: []params.ConfigSettingsResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Settings: params.ConfigSettings{"blog-title": "My Title"}},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterBaseSuite) testWatchServiceRelations(
	c *gc.C,
	facade interface {
		WatchServiceRelations(args params.Entities) (params.StringsWatchResults, error)
	},
) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "service-mysql"},
		{Tag: "service-wordpress"},
		{Tag: "service-foo"},
	}}
	result, err := facade.WatchServiceRelations(args)
	s.assertOneStringsWatcher(c, result, err)
}

func (s *uniterBaseSuite) testCharmArchiveSha256(
	c *gc.C,
	facade interface {
		CharmArchiveSha256(args params.CharmURLs) (params.StringResults, error)
	},
) {
	dummyCharm := s.AddTestingCharm(c, "dummy")

	args := params.CharmURLs{URLs: []params.CharmURL{
		{URL: "something-invalid"},
		{URL: s.wpCharm.String()},
		{URL: dummyCharm.String()},
	}}
	result, err := facade.CharmArchiveSha256(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: s.wpCharm.BundleSha256()},
			{Result: dummyCharm.BundleSha256()},
		},
	})
}

func (s *uniterBaseSuite) testCurrentEnvironUUID(
	c *gc.C,
	facade interface {
		CurrentEnvironUUID() (params.StringResult, error)
	},
) {
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)

	result, err := facade.CurrentEnvironUUID()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResult{Result: env.UUID()})
}

func (s *uniterBaseSuite) testCurrentEnvironment(
	c *gc.C,
	facade interface {
		CurrentEnvironment() (params.EnvironmentResult, error)
	},
) {
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)

	result, err := facade.CurrentEnvironment()
	c.Assert(err, gc.IsNil)
	expected := params.EnvironmentResult{
		Name: env.Name(),
		UUID: env.UUID(),
	}
	c.Assert(result, gc.DeepEquals, expected)
}

type actions interface {
	Actions(args params.Entities) (params.ActionsQueryResults, error)
}

func (s *uniterBaseSuite) testActions(c *gc.C, facade actions) {
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
		results, err := facade.Actions(args)
		c.Assert(err, gc.IsNil)
		c.Assert(results.Results, gc.HasLen, 1)

		actionsQueryResult := results.Results[0]

		c.Assert(actionsQueryResult.Error, gc.IsNil)
		c.Assert(actionsQueryResult.Action, jc.DeepEquals, actionTest.action)
	}
}

func (s *uniterBaseSuite) testActionsNotPresent(c *gc.C, facade actions) {
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: names.JoinActionTag("wordpress/0", 0).String(),
		}},
	}
	results, err := facade.Actions(args)
	c.Assert(err, gc.IsNil)

	c.Assert(results.Results, gc.HasLen, 1)
	actionsQueryResult := results.Results[0]
	c.Assert(actionsQueryResult.Error, gc.NotNil)
	c.Assert(actionsQueryResult.Error, gc.ErrorMatches, `action .*wordpress/0[^0-9]+0[^0-9]+ not found`)
}

func (s *uniterBaseSuite) testActionsWrongUnit(
	c *gc.C,
	factory func(_ *state.State, _ *common.Resources, _ common.Authorizer) (actions, error),
) {
	// Action doesn't match unit.
	mysqlUnitAuthorizer := apiservertesting.FakeAuthorizer{
		Tag: s.mysqlUnit.Tag(),
	}
	mysqlUnitFacade, err := factory(s.State, s.resources, mysqlUnitAuthorizer)
	c.Assert(err, gc.IsNil)

	args := params.Entities{
		Entities: []params.Entity{{
			Tag: names.JoinActionTag("wordpress/0", 0).String(),
		}},
	}
	actions, err := mysqlUnitFacade.Actions(args)
	c.Assert(err, gc.IsNil)
	c.Assert(len(actions.Results), gc.Equals, 1)
	c.Assert(actions.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *uniterBaseSuite) testActionsPermissionDenied(c *gc.C, facade actions) {
	// Same unit, but not one that has access.
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: names.JoinActionTag("mysql/0", 0).String(),
		}},
	}
	actions, err := facade.Actions(args)
	c.Assert(err, gc.IsNil)
	c.Assert(len(actions.Results), gc.Equals, 1)
	c.Assert(actions.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

type finishActions interface {
	FinishActions(args params.ActionExecutionResults) (params.ErrorResults, error)
}

func (s *uniterBaseSuite) testFinishActionsSuccess(c *gc.C, facade finishActions) {
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
	res, err := facade.FinishActions(actionResults)
	c.Assert(err, gc.IsNil)
	c.Assert(res, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}})

	results, err = s.wordpressUnit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(len(results), gc.Equals, 1)
	c.Assert(results[0].Status(), gc.Equals, state.ActionCompleted)
	res2, errstr := results[0].Results()
	c.Assert(errstr, gc.Equals, "")
	c.Assert(res2, gc.DeepEquals, testOutput)
	c.Assert(results[0].Name(), gc.Equals, testName)
}

func (s *uniterBaseSuite) testFinishActionsFailure(c *gc.C, facade finishActions) {
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
	res, err := facade.FinishActions(actionResults)
	c.Assert(err, gc.IsNil)
	c.Assert(res, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}})

	results, err = s.wordpressUnit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(len(results), gc.Equals, 1)
	c.Assert(results[0].Status(), gc.Equals, state.ActionFailed)
	res2, errstr := results[0].Results()
	c.Assert(errstr, gc.Equals, testError)
	c.Assert(res2, gc.DeepEquals, map[string]interface{}{})
	c.Assert(results[0].Name(), gc.Equals, testName)
}

func (s *uniterBaseSuite) testFinishActionsAuthAccess(c *gc.C, facade finishActions) {
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
	res, err := facade.FinishActions(actionResults)
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

func (s *uniterBaseSuite) testRelation(
	c *gc.C,
	facade interface {
		Relation(args params.RelationUnits) (params.RelationResults, error)
	},
) {
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
	result, err := facade.Relation(args)
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

func (s *uniterBaseSuite) testRelationById(
	c *gc.C,
	facade interface {
		RelationById(args params.RelationIds) (params.RelationResults, error)
	},
) {
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
	result, err := facade.RelationById(args)
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

func (s *uniterBaseSuite) testProviderType(
	c *gc.C,
	facade interface {
		ProviderType() (params.StringResult, error)
	},
) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)

	result, err := facade.ProviderType()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResult{Result: cfg.Type()})
}

func (s *uniterBaseSuite) testEnterScope(
	c *gc.C,
	facade interface {
		EnterScope(args params.RelationUnits) (params.ErrorResults, error)
	},
) {
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
	result, err := facade.EnterScope(args)
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

func (s *uniterBaseSuite) testLeaveScope(
	c *gc.C,
	facade interface {
		LeaveScope(args params.RelationUnits) (params.ErrorResults, error)
	},
) {
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
	result, err := facade.LeaveScope(args)
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

func (s *uniterBaseSuite) testJoinedRelations(
	c *gc.C,
	facade interface {
		JoinedRelations(args params.Entities) (params.StringsResults, error)
	},
) {
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
		result, err := facade.JoinedRelations(args)
		c.Assert(err, gc.IsNil)
		c.Assert(result, gc.DeepEquals, expect)
	}
	check()
	err = relUnit.PrepareLeaveScope()
	c.Assert(err, gc.IsNil)
	check()
}

type readSettings interface {
	ReadSettings(args params.RelationUnits) (params.RelationSettingsResults, error)
}

func (s *uniterBaseSuite) testReadSettings(c *gc.C, facade readSettings) {
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
	result, err := facade.ReadSettings(args)
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

func (s *uniterBaseSuite) testReadSettingsWithNonStringValuesFails(c *gc.C, facade readSettings) {
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
	result, err := facade.ReadSettings(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.RelationSettingsResults{
		Results: []params.RelationSettingsResult{
			{Error: &params.Error{Message: expectErr}},
		},
	})
}

type readRemoteSettings interface {
	ReadRemoteSettings(args params.RelationUnitPairs) (params.RelationSettingsResults, error)
}

func (s *uniterBaseSuite) testReadRemoteSettings(c *gc.C, facade readRemoteSettings) {
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
	result, err := facade.ReadRemoteSettings(args)

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
	result, err = facade.ReadRemoteSettings(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, expect)

	// Now destroy the remote unit, and check its settings can still be read.
	err = s.mysqlUnit.Destroy()
	c.Assert(err, gc.IsNil)
	err = s.mysqlUnit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.mysqlUnit.Remove()
	c.Assert(err, gc.IsNil)
	result, err = facade.ReadRemoteSettings(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, expect)
}

func (s *uniterBaseSuite) testReadRemoteSettingsWithNonStringValuesFails(c *gc.C, facade readRemoteSettings) {
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
	result, err := facade.ReadRemoteSettings(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.RelationSettingsResults{
		Results: []params.RelationSettingsResult{
			{Error: &params.Error{Message: expectErr}},
		},
	})
}

func (s *uniterBaseSuite) testUpdateSettings(
	c *gc.C,
	facade interface {
		UpdateSettings(args params.RelationUnitsSettings) (params.ErrorResults, error)
	},
) {
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
	result, err := facade.UpdateSettings(args)
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

func (s *uniterBaseSuite) testWatchRelationUnits(
	c *gc.C,
	facade interface {
		WatchRelationUnits(args params.RelationUnits) (params.RelationUnitsWatchResults, error)
	},
) {
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
	result, err := facade.WatchRelationUnits(args)
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

func (s *uniterBaseSuite) testAPIAddresses(
	c *gc.C,
	facade interface {
		APIAddresses() (params.StringsResult, error)
	},
) {
	hostPorts := [][]network.HostPort{{{
		Address: network.NewAddress("0.1.2.3", network.ScopeUnknown),
		Port:    1234,
	}}}

	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, gc.IsNil)

	result, err := facade.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResult{
		Result: []string{"0.1.2.3:1234"},
	})
}

func (s *uniterBaseSuite) testWatchUnitAddresses(
	c *gc.C,
	facade interface {
		WatchUnitAddresses(args params.Entities) (params.NotifyWatchResults, error)
	},
) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-0"},
		{Tag: "service-wordpress"},
	}}
	result, err := facade.WatchUnitAddresses(args)
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

type addMetrics interface {
	AddMetrics(args params.MetricsParams) (params.ErrorResults, error)
}

func (s *uniterBaseSuite) testAddMetrics(c *gc.C, facade addMetrics) {
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
	result, err := facade.AddMetrics(args)
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

func (s *uniterBaseSuite) testAddMetricsIncorrectTag(c *gc.C, facade addMetrics) {
	now := time.Now()

	tags := []string{"user-admin", "unit-nosuchunit", "thisisnotatag", "machine-0", "environment-blah"}

	for _, tag := range tags {

		args := params.MetricsParams{
			Metrics: []params.MetricsParam{{
				Tag:     tag,
				Metrics: []params.Metric{{"A", "5", now}, {"B", "0.71", now}},
			}},
		}

		result, err := facade.AddMetrics(args)
		c.Assert(err, gc.IsNil)
		c.Assert(result.Results, gc.HasLen, 1)
		c.Assert(result.Results[0].Error, gc.ErrorMatches, "permission denied")
		metrics, err := s.State.MetricBatches()
		c.Assert(err, gc.IsNil)
		c.Assert(metrics, gc.HasLen, 0)
	}
}

func (s *uniterBaseSuite) testAddMetricsUnauthenticated(c *gc.C, facade addMetrics) {
	now := time.Now()
	args := params.MetricsParams{
		Metrics: []params.MetricsParam{{
			Tag:     s.mysqlUnit.Tag().String(),
			Metrics: []params.Metric{{"A", "5", now}, {"B", "0.71", now}},
		}},
	}
	result, err := facade.AddMetrics(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "permission denied")
	metrics, err := s.State.MetricBatches()
	c.Assert(err, gc.IsNil)
	c.Assert(metrics, gc.HasLen, 0)
}

type getMeterStatus interface {
	GetMeterStatus(args params.Entities) (params.MeterStatusResults, error)
}

func (s *uniterBaseSuite) testGetMeterStatus(c *gc.C, facade getMeterStatus) {
	args := params.Entities{Entities: []params.Entity{{Tag: s.wordpressUnit.Tag().String()}}}
	result, err := facade.GetMeterStatus(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Code, gc.Equals, "NOT SET")
	c.Assert(result.Results[0].Info, gc.Equals, "")

	newCode := "GREEN"
	newInfo := "All is ok."

	err = s.wordpressUnit.SetMeterStatus(newCode, newInfo)
	c.Assert(err, gc.IsNil)

	result, err = facade.GetMeterStatus(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Code, gc.DeepEquals, newCode)
	c.Assert(result.Results[0].Info, gc.DeepEquals, newInfo)
}

func (s *uniterBaseSuite) testGetMeterStatusUnauthenticated(c *gc.C, facade getMeterStatus) {
	args := params.Entities{Entities: []params.Entity{{s.mysqlUnit.Tag().String()}}}
	result, err := facade.GetMeterStatus(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "permission denied")
	c.Assert(result.Results[0].Code, gc.Equals, "")
	c.Assert(result.Results[0].Info, gc.Equals, "")
}

func (s *uniterBaseSuite) testGetMeterStatusBadTag(c *gc.C, facade getMeterStatus) {
	tags := []string{
		"user-admin",
		"unit-nosuchunit",
		"thisisnotatag",
		"machine-0",
		"environment-blah",
	}
	args := params.Entities{Entities: make([]params.Entity, len(tags))}
	for i, tag := range tags {
		args.Entities[i] = params.Entity{Tag: tag}
	}
	result, err := facade.GetMeterStatus(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, len(tags))
	for i, result := range result.Results {
		c.Logf("checking result %d", i)
		c.Assert(result.Code, gc.Equals, "")
		c.Assert(result.Info, gc.Equals, "")
		c.Assert(result.Error, gc.ErrorMatches, "permission denied")
	}
}

func (s *uniterBaseSuite) testWatchMeterStatus(
	c *gc.C,
	facade interface {
		WatchMeterStatus(args params.Entities) (params.NotifyWatchResults, error)
	},
) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := facade.WatchMeterStatus(args)
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

	err = s.wordpressUnit.SetMeterStatus("GREEN", "No additional information.")
	wc.AssertOneChange()
}

func (s *uniterBaseSuite) assertOneStringsWatcher(c *gc.C, result params.StringsWatchResults, err error) {
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

func (s *uniterBaseSuite) assertInScope(c *gc.C, relUnit *state.RelationUnit, inScope bool) {
	ok, err := relUnit.InScope()
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, inScope)
}

func (s *uniterBaseSuite) addRelation(c *gc.C, first, second string) *state.Relation {
	eps, err := s.State.InferEndpoints(first, second)
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	return rel
}

func (s *uniterBaseSuite) addRelatedService(c *gc.C, firstSvc, relatedSvc string, unit *state.Unit) (*state.Relation, *state.Service, *state.Unit) {
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
