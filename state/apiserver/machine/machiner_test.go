// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/apiserver/machine"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	statetesting "launchpad.net/juju-core/state/testing"
)

type machinerSuite struct {
	commonSuite

	resources *common.Resources
	machiner  *machine.MachinerAPI
}

var _ = gc.Suite(&machinerSuite{})

func (s *machinerSuite) SetUpTest(c *gc.C) {
	s.commonSuite.SetUpTest(c)

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()

	// Create a machiner API for machine 1.
	machiner, err := machine.NewMachinerAPI(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, gc.IsNil)
	s.machiner = machiner
}

func (s *machinerSuite) TestMachinerFailsWithNonMachineAgentUser(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.MachineAgent = false
	aMachiner, err := machine.NewMachinerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.NotNil)
	c.Assert(aMachiner, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *machinerSuite) TestSetStatus(c *gc.C) {
	err := s.machine0.SetStatus(params.StatusStarted, "blah", nil)
	c.Assert(err, gc.IsNil)
	err = s.machine1.SetStatus(params.StatusStopped, "foo", nil)
	c.Assert(err, gc.IsNil)

	args := params.SetStatus{
		Entities: []params.SetEntityStatus{
			{Tag: "machine-1", Status: params.StatusError, Info: "not really"},
			{Tag: "machine-0", Status: params.StatusStopped, Info: "foobar"},
			{Tag: "machine-42", Status: params.StatusStarted, Info: "blah"},
		}}
	result, err := s.machiner.SetStatus(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify machine 0 - no change.
	status, info, _, err := s.machine0.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStarted)
	c.Assert(info, gc.Equals, "blah")
	// ...machine 1 is fine though.
	status, info, _, err = s.machine1.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusError)
	c.Assert(info, gc.Equals, "not really")
}

func (s *machinerSuite) TestLife(c *gc.C) {
	err := s.machine1.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.machine1.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.machine1.Life(), gc.Equals, state.Dead)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	result, err := s.machiner.Life(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "dead"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *machinerSuite) TestEnsureDead(c *gc.C) {
	c.Assert(s.machine0.Life(), gc.Equals, state.Alive)
	c.Assert(s.machine1.Life(), gc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	result, err := s.machiner.EnsureDead(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	err = s.machine0.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.machine0.Life(), gc.Equals, state.Alive)
	err = s.machine1.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.machine1.Life(), gc.Equals, state.Dead)

	// Try it again on a Dead machine; should work.
	args = params.Entities{
		Entities: []params.Entity{{Tag: "machine-1"}},
	}
	result, err = s.machiner.EnsureDead(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{nil}},
	})

	// Verify Life is unchanged.
	err = s.machine1.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.machine1.Life(), gc.Equals, state.Dead)
}

func (s *machinerSuite) TestSetMachineAddresses(c *gc.C) {
	c.Assert(s.machine0.Addresses(), gc.HasLen, 0)
	c.Assert(s.machine1.Addresses(), gc.HasLen, 0)

	addresses := []instance.Address{
		instance.NewAddress("127.0.0.1"),
		instance.NewAddress("8.8.8.8"),
	}

	args := params.SetMachinesAddresses{MachineAddresses: []params.MachineAddresses{
		{Tag: "machine-1", Addresses: addresses},
		{Tag: "machine-0", Addresses: addresses},
		{Tag: "machine-42", Addresses: addresses},
	}}

	result, err := s.machiner.SetMachineAddresses(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	err = s.machine1.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.machine1.MachineAddresses(), gc.DeepEquals, addresses)
	err = s.machine0.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.machine0.MachineAddresses(), gc.HasLen, 0)
}

func (s *machinerSuite) TestWatch(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	result, err := s.machiner.Watch(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	c.Assert(result.Results[0].NotifyWatcherId, gc.Equals, "1")
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, s.State, resource.(state.NotifyWatcher))
	wc.AssertNoChange()
}
