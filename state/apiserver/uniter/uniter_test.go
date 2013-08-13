// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	"launchpad.net/juju-core/state/apiserver/uniter"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
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
	mysql         *state.Service
	wordpressUnit *state.Unit
	mysqlUnit     *state.Unit

	uniter *uniter.UniterAPI
}

var _ = gc.Suite(&uniterSuite{})

func (s *uniterSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create two machines, two services and add a unit to each service.
	var err error
	s.machine0, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	s.machine1, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	s.wordpress, err = s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
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
	err := s.wordpressUnit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Dead)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.Life(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "dead"},
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

func (s *uniterSuite) TestWatch(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.Watch(args)
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
	c.Assert(result.Results[1].NotifyWatcherId, gc.Equals, "1")
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, s.State, resource.(state.NotifyWatcher))
	wc.AssertNoChange()
}
