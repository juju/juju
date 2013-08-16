// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	"launchpad.net/juju-core/state/apiserver/uniter"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type uniterSuite struct {
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

	uniter *uniter.UniterAPI
}

var _ = gc.Suite(&uniterSuite{})

func (s *uniterSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.wpCharm = s.AddTestingCharm(c, "wordpress")
	// Create two machines, two services and add a unit to each service.
	var err error
	s.machine0, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	s.machine1, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	s.wordpress, err = s.State.AddService("wordpress", s.wpCharm)
	c.Assert(err, gc.IsNil)
	s.mysql, err = s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, gc.IsNil)
	s.wordpressUnit, err = s.wordpress.AddUnit()
	c.Assert(err, gc.IsNil)
	s.mysqlUnit, err = s.mysql.AddUnit()
	c.Assert(err, gc.IsNil)
	// Assign each unit to each machine.
	err = s.wordpressUnit.AssignToMachine(s.machine0)
	c.Assert(err, gc.IsNil)
	err = s.mysqlUnit.AssignToMachine(s.machine1)
	c.Assert(err, gc.IsNil)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming unit 0 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:       s.wordpressUnit.Tag(),
		LoggedIn:  true,
		Manager:   false,
		UnitAgent: true,
		Entity:    s.wordpressUnit,
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()

	// Create a uniter API for unit 0.
	s.uniter, err = uniter.NewUniterAPI(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, gc.IsNil)
}

func (s *uniterSuite) TestUniterFailsWithNonUnitAgentUser(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.UnitAgent = false
	anUniter, err := uniter.NewUniterAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.NotNil)
	c.Assert(anUniter, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *uniterSuite) TestSetStatus(c *gc.C) {
	err := s.wordpressUnit.SetStatus(params.StatusStarted, "blah")
	c.Assert(err, gc.IsNil)
	err = s.mysqlUnit.SetStatus(params.StatusStopped, "foo")
	c.Assert(err, gc.IsNil)

	args := params.SetStatus{
		Entities: []params.SetEntityStatus{
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
	status, info, err := s.mysqlUnit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStopped)
	c.Assert(info, gc.Equals, "foo")
	// ...wordpressUnit is fine though.
	status, info, err = s.wordpressUnit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStopped)
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

	// Check relation life as well.
	c.Assert(rel.Refresh(), gc.IsNil)
	c.Assert(rel.Life(), gc.Equals, state.Dying)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "service-mysql"},
		{Tag: "service-wordpress"},
		{Tag: "service-foo"},
		{Tag: "just-foo"},
		{Tag: rel.Tag()},
		{Tag: "relation-42"},
		{Tag: "relation-blah"},
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
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "dying"},
			{Error: &params.Error{
				Code:    "not found",
				Message: "relation 42 not found"}},
			{Error: apiservertesting.ErrUnauthorized},
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
		{Tag: "just-foo"},
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
			{Error: apiservertesting.ErrUnauthorized},
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
	err := s.wordpressUnit.SetPublicAddress("1.2.3.4")
	c.Assert(err, gc.IsNil)
	address, ok := s.wordpressUnit.PublicAddress()
	c.Assert(address, gc.Equals, "1.2.3.4")
	c.Assert(ok, jc.IsTrue)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.PublicAddress(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringBoolResults{
		Results: []params.StringBoolResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "1.2.3.4", Ok: true},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestSetPublicAddress(c *gc.C) {
	err := s.wordpressUnit.SetPublicAddress("1.2.3.4")
	c.Assert(err, gc.IsNil)
	address, ok := s.wordpressUnit.PublicAddress()
	c.Assert(address, gc.Equals, "1.2.3.4")
	c.Assert(ok, jc.IsTrue)

	args := params.SetEntityAddresses{Entities: []params.SetEntityAddress{
		{Tag: "unit-mysql-0", Address: "4.3.2.1"},
		{Tag: "unit-wordpress-0", Address: "4.4.2.2"},
		{Tag: "unit-foo-42", Address: "2.2.4.4"},
	}}
	result, err := s.uniter.SetPublicAddress(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify wordpressUnit's address has changed.
	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	address, ok = s.wordpressUnit.PublicAddress()
	c.Assert(address, gc.Equals, "4.4.2.2")
	c.Assert(ok, jc.IsTrue)
}

func (s *uniterSuite) TestPrivateAddress(c *gc.C) {
	err := s.wordpressUnit.SetPrivateAddress("1.2.3.4")
	c.Assert(err, gc.IsNil)
	address, ok := s.wordpressUnit.PrivateAddress()
	c.Assert(address, gc.Equals, "1.2.3.4")
	c.Assert(ok, jc.IsTrue)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.PrivateAddress(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringBoolResults{
		Results: []params.StringBoolResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "1.2.3.4", Ok: true},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestSetPrivateAddress(c *gc.C) {
	err := s.wordpressUnit.SetPrivateAddress("1.2.3.4")
	c.Assert(err, gc.IsNil)
	address, ok := s.wordpressUnit.PrivateAddress()
	c.Assert(address, gc.Equals, "1.2.3.4")
	c.Assert(ok, jc.IsTrue)

	args := params.SetEntityAddresses{Entities: []params.SetEntityAddress{
		{Tag: "unit-mysql-0", Address: "4.3.2.1"},
		{Tag: "unit-wordpress-0", Address: "4.4.2.2"},
		{Tag: "unit-foo-42", Address: "2.2.4.4"},
	}}
	result, err := s.uniter.SetPrivateAddress(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify wordpressUnit's address has changed.
	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	address, ok = s.wordpressUnit.PrivateAddress()
	c.Assert(address, gc.Equals, "4.4.2.2")
	c.Assert(ok, jc.IsTrue)
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
	_, subordinate := s.addRelatedService(c, "wordpress", "logging", s.wordpressUnit)

	principal, ok := subordinate.PrincipalName()
	c.Assert(principal, gc.Equals, s.wordpressUnit.Name())
	c.Assert(ok, jc.IsTrue)

	// First try it as wordpressUnit's agent.
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: subordinate.Tag()},
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
			{Result: "wordpress/0", Ok: true, Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) addRelatedService(c *gc.C, firstSvc, relatedSvc string, unit *state.Unit) (*state.Service, *state.Unit) {
	relatedService, err := s.State.AddService(relatedSvc, s.AddTestingCharm(c, relatedSvc))
	c.Assert(err, gc.IsNil)
	rel := s.addRelation(c, firstSvc, relatedSvc)
	relUnit, err := rel.Unit(unit)
	c.Assert(err, gc.IsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	relatedUnit, err := relatedService.Unit(relatedSvc + "/0")
	c.Assert(err, gc.IsNil)
	return relatedService, relatedUnit
}

func (s *uniterSuite) TestSubordinateNames(c *gc.C) {
	// Add two subordinates to wordpressUnit.
	s.addRelatedService(c, "wordpress", "logging", s.wordpressUnit)
	s.addRelatedService(c, "wordpress", "monitoring", s.wordpressUnit)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-logging-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.SubordinateNames(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: []string{"logging/0", "monitoring/0"}},
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
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
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
		{Tag: "just-foo"},
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
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestSetCharmURL(c *gc.C) {
	charmUrl, ok := s.wordpressUnit.CharmURL()
	c.Assert(charmUrl, gc.IsNil)
	c.Assert(ok, jc.IsFalse)

	args := params.EntitiesCharmURL{Entities: []params.EntityCharmURL{
		{Tag: "unit-mysql-0", CharmURL: "cs:series/service-42"},
		{Tag: "unit-wordpress-0", CharmURL: s.wpCharm.String()},
		{Tag: "unit-foo-42", CharmURL: "cs:series/foo-321"},
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
	charmUrl, ok = s.wordpressUnit.CharmURL()
	c.Assert(charmUrl, gc.NotNil)
	c.Assert(charmUrl.String(), gc.Equals, s.wpCharm.String())
	c.Assert(ok, jc.IsTrue)
}

func (s *uniterSuite) TestOpenPort(c *gc.C) {
	openedPorts := s.wordpressUnit.OpenedPorts()
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
	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	openedPorts = s.wordpressUnit.OpenedPorts()
	c.Assert(openedPorts, gc.DeepEquals, []instance.Port{
		{Protocol: "udp", Number: 4321},
	})
}

func (s *uniterSuite) TestClosePort(c *gc.C) {
	// Open port udp:4321 in advance on wordpressUnit.
	err := s.wordpressUnit.OpenPort("udp", 4321)
	c.Assert(err, gc.IsNil)
	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	openedPorts := s.wordpressUnit.OpenedPorts()
	c.Assert(openedPorts, gc.DeepEquals, []instance.Port{
		{Protocol: "udp", Number: 4321},
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
	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	openedPorts = s.wordpressUnit.OpenedPorts()
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
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Settings: params.Settings{"blog-title": "My Title"}},
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

func (s *uniterSuite) TestCharmBundleURL(c *gc.C) {
	dummyCharm := s.AddTestingCharm(c, "dummy")

	args := params.CharmURLs{URLs: []params.CharmURL{
		{URL: "something-invalid"},
		{URL: s.wpCharm.String()},
		{URL: dummyCharm.String()},
	}}
	result, err := s.uniter.CharmBundleURL(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: s.wpCharm.BundleURL().String()},
			{Result: dummyCharm.BundleURL().String()},
		},
	})
}

func (s *uniterSuite) TestCharmBundleSha256(c *gc.C) {
	dummyCharm := s.AddTestingCharm(c, "dummy")

	args := params.CharmURLs{URLs: []params.CharmURL{
		{URL: "something-invalid"},
		{URL: s.wpCharm.String()},
		{URL: dummyCharm.String()},
	}}
	result, err := s.uniter.CharmBundleSha256(args)
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

func (s *uniterSuite) addRelation(c *gc.C, first, second string) *state.Relation {
	eps, err := s.State.InferEndpoints([]string{first, second})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	return rel
}

func (s *uniterSuite) TestRelation(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	c.Assert(rel.Id(), gc.Equals, 0)
	wpEp, err := rel.Endpoint("wordpress")
	c.Assert(err, gc.IsNil)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-42", Unit: "unit-foo-0"},
		{Relation: "relation-0", Unit: "unit-wordpress-0"},
		{Relation: "relation-0", Unit: "unit-mysql-0"},
		{Relation: "relation-0", Unit: "unit-foo-0"},
		{Relation: "relation-blah", Unit: "unit-wordpress-0"},
		{Relation: "service-foo", Unit: "user-admin"},
		{Relation: "foo", Unit: "bar"},
		{Relation: "unit-wordpress-0", Unit: "relation-0"},
	}}
	result, err := s.uniter.Relation(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.RelationResults{
		Results: []params.RelationResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				Id:  rel.Id(),
				Key: rel.String(),
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

func (s *uniterSuite) TestProviderType(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)

	result, err := s.uniter.ProviderType()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringResult{Result: cfg.Type()})
}

func (s *uniterSuite) TestEnterScope(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, gc.IsNil)
	c.Assert(relUnit.InScope(), jc.IsFalse)

	settings := params.Settings{
		"some": "settings",
	}
	args := params.RelationUnitsSettings{RelationUnits: []params.RelationUnitSettings{
		{Relation: "relation-42", Unit: "unit-foo-0", Settings: nil},
		{Relation: "relation-0", Unit: "unit-wordpress-0", Settings: settings},
		{Relation: "relation-0", Unit: "unit-wordpress-0", Settings: nil},
		{Relation: "relation-42", Unit: "unit-wordpress-0", Settings: nil},
		{Relation: "relation-foo", Unit: "unit-wordpress-0", Settings: nil},
		{Relation: "service-wordpress", Unit: "unit-foo-0", Settings: nil},
		{Relation: "foo", Unit: "bar", Settings: nil},
		{Relation: "relation-0", Unit: "unit-mysql-0", Settings: nil},
		{Relation: "relation-0", Unit: "service-wordpress", Settings: nil},
		{Relation: "relation-0", Unit: "service-mysql", Settings: nil},
		{Relation: "relation-0", Unit: "user-admin", Settings: nil},
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
	c.Assert(relUnit.InScope(), jc.IsTrue)
	readSettings, err := relUnit.ReadSettings(s.wordpressUnit.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(params.Settings(readSettings), gc.DeepEquals, settings)
}

func (s *uniterSuite) TestLeaveScope(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, gc.IsNil)
	settings := params.Settings{
		"some": "settings",
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, gc.IsNil)
	c.Assert(relUnit.InScope(), jc.IsTrue)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-42", Unit: "unit-foo-0"},
		{Relation: "relation-0", Unit: "unit-wordpress-0"},
		{Relation: "relation-0", Unit: "unit-wordpress-0"},
		{Relation: "relation-42", Unit: "unit-wordpress-0"},
		{Relation: "relation-foo", Unit: "unit-wordpress-0"},
		{Relation: "service-wordpress", Unit: "unit-foo-0"},
		{Relation: "foo", Unit: "bar"},
		{Relation: "relation-0", Unit: "unit-mysql-0"},
		{Relation: "relation-0", Unit: "service-wordpress"},
		{Relation: "relation-0", Unit: "service-mysql"},
		{Relation: "relation-0", Unit: "user-admin"},
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
	c.Assert(relUnit.InScope(), jc.IsFalse)
	readSettings, err := relUnit.ReadSettings(s.wordpressUnit.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(params.Settings(readSettings), gc.DeepEquals, settings)
}

func (s *uniterSuite) TestReadSettings(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, gc.IsNil)
	settings := params.Settings{
		"some": "settings",
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, gc.IsNil)
	c.Assert(relUnit.InScope(), jc.IsTrue)

	// First test most of the invalid args tests and two valid:
	// 1. Reading local unit settings successfully;
	// 2. Trying to read the (unset) remote unit settings.
	args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{
		{Relation: "relation-42", LocalUnit: "unit-foo-0", RemoteUnit: "foo"},
		{Relation: "relation-0", LocalUnit: "unit-wordpress-0", RemoteUnit: "unit-wordpress-0"},
		{Relation: "relation-0", LocalUnit: "unit-wordpress-0", RemoteUnit: "unit-mysql-0"},
		{Relation: "relation-42", LocalUnit: "unit-wordpress-0", RemoteUnit: ""},
		{Relation: "relation-foo", LocalUnit: "", RemoteUnit: ""},
		{Relation: "service-wordpress", LocalUnit: "unit-foo-0", RemoteUnit: "user-admin"},
		{Relation: "foo", LocalUnit: "bar", RemoteUnit: "baz"},
		{Relation: "relation-0", LocalUnit: "unit-mysql-0", RemoteUnit: "unit-wordpress-0"},
		{Relation: "relation-0", LocalUnit: "service-wordpress", RemoteUnit: "service-mysql"},
		{Relation: "relation-0", LocalUnit: "service-mysql", RemoteUnit: "foo"},
		{Relation: "relation-0", LocalUnit: "user-admin", RemoteUnit: "unit-wordpress-0"},
	}}
	result, err := s.uniter.ReadSettings(args)
	// We don't set the remote unit settings on purpose to test the error.
	expectErr := `cannot read settings for unit "mysql/0" in relation "wordpress:db mysql:server": settings not found`
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Settings: settings},
			{Error: &params.Error{Message: expectErr}},
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
	settings = params.Settings{
		"other": "things",
	}
	err = relUnit.LeaveScope()
	c.Assert(err, gc.IsNil)
	c.Assert(relUnit.InScope(), jc.IsFalse)
	err = relUnit.EnterScope(settings)
	c.Assert(err, gc.IsNil)
	c.Assert(relUnit.InScope(), jc.IsTrue)

	// Test the remote unit settings can be read.
	args = params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{{
		Relation:   "relation-0",
		LocalUnit:  "unit-wordpress-0",
		RemoteUnit: "unit-mysql-0",
	}}}
	result, err = s.uniter.ReadSettings(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: settings},
		},
	})
}

func (s *uniterSuite) TestWriteSettings(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, gc.IsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(relUnit.InScope(), jc.IsTrue)
	settings := params.Settings{
		"some": "settings",
	}

	args := params.RelationUnitsSettings{RelationUnits: []params.RelationUnitSettings{
		{Relation: "relation-42", Unit: "unit-foo-0", Settings: nil},
		{Relation: "relation-0", Unit: "unit-wordpress-0", Settings: settings},
		{Relation: "relation-42", Unit: "unit-wordpress-0", Settings: nil},
		{Relation: "relation-foo", Unit: "unit-wordpress-0", Settings: nil},
		{Relation: "service-wordpress", Unit: "unit-foo-0", Settings: nil},
		{Relation: "foo", Unit: "bar", Settings: nil},
		{Relation: "relation-0", Unit: "unit-mysql-0", Settings: nil},
		{Relation: "relation-0", Unit: "service-wordpress", Settings: nil},
		{Relation: "relation-0", Unit: "service-mysql", Settings: nil},
		{Relation: "relation-0", Unit: "user-admin", Settings: nil},
	}}
	result, err := s.uniter.WriteSettings(args)
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
	c.Assert(relUnit.InScope(), jc.IsTrue)
	readSettings, err := relUnit.ReadSettings(s.wordpressUnit.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(params.Settings(readSettings), gc.DeepEquals, settings)
}
