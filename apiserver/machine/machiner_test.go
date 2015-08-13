// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/machine"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	statetesting "github.com/juju/juju/state/testing"
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
	c.Assert(err, jc.ErrorIsNil)
	s.machiner = machiner
}

func (s *machinerSuite) TestMachinerFailsWithNonMachineAgentUser(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("ubuntu/1")
	aMachiner, err := machine.NewMachinerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.NotNil)
	c.Assert(aMachiner, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *machinerSuite) TestSetStatus(c *gc.C) {
	err := s.machine0.SetStatus(state.StatusStarted, "blah", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine1.SetStatus(state.StatusStopped, "foo", nil)
	c.Assert(err, jc.ErrorIsNil)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "machine-1", Status: params.StatusError, Info: "not really"},
			{Tag: "machine-0", Status: params.StatusStopped, Info: "foobar"},
			{Tag: "machine-42", Status: params.StatusStarted, Info: "blah"},
		}}
	result, err := s.machiner.SetStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify machine 0 - no change.
	statusInfo, err := s.machine0.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusStarted)
	c.Assert(statusInfo.Message, gc.Equals, "blah")
	// ...machine 1 is fine though.
	statusInfo, err = s.machine1.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusError)
	c.Assert(statusInfo.Message, gc.Equals, "not really")
}

func (s *machinerSuite) TestLife(c *gc.C) {
	err := s.machine1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine1.Life(), gc.Equals, state.Dead)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	result, err := s.machiner.Life(args)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	err = s.machine0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine0.Life(), gc.Equals, state.Alive)
	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine1.Life(), gc.Equals, state.Dead)

	// Try it again on a Dead machine; should work.
	args = params.Entities{
		Entities: []params.Entity{{Tag: "machine-1"}},
	}
	result, err = s.machiner.EnsureDead(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{nil}},
	})

	// Verify Life is unchanged.
	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine1.Life(), gc.Equals, state.Dead)
}

func (s *machinerSuite) TestSetMachineAddresses(c *gc.C) {
	c.Assert(s.machine0.Addresses(), gc.HasLen, 0)
	c.Assert(s.machine1.Addresses(), gc.HasLen, 0)

	addresses := network.NewAddresses("127.0.0.1", "8.8.8.8")

	args := params.SetMachinesAddresses{MachineAddresses: []params.MachineAddresses{
		{Tag: "machine-1", Addresses: params.FromNetworkAddresses(addresses)},
		{Tag: "machine-0", Addresses: params.FromNetworkAddresses(addresses)},
		{Tag: "machine-42", Addresses: params.FromNetworkAddresses(addresses)},
	}}

	result, err := s.machiner.SetMachineAddresses(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	expectedAddresses := network.NewAddresses("8.8.8.8", "127.0.0.1")
	c.Assert(s.machine1.MachineAddresses(), gc.DeepEquals, expectedAddresses)
	err = s.machine0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine0.MachineAddresses(), gc.HasLen, 0)
}

func (s *machinerSuite) TestSetEmptyMachineAddresses(c *gc.C) {
	// Set some addresses so we can ensure they are removed.
	addresses := network.NewAddresses("127.0.0.1", "8.8.8.8")
	args := params.SetMachinesAddresses{MachineAddresses: []params.MachineAddresses{
		{Tag: "machine-1", Addresses: params.FromNetworkAddresses(addresses)},
	}}
	result, err := s.machiner.SetMachineAddresses(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
		},
	})
	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine1.MachineAddresses(), gc.HasLen, 2)

	args.MachineAddresses[0].Addresses = nil
	result, err = s.machiner.SetMachineAddresses(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
		},
	})

	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine1.MachineAddresses(), gc.HasLen, 0)
}

func (s *machinerSuite) TestJobs(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}

	result, err := s.machiner.Jobs(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.JobsResults{
		Results: []params.JobsResult{
			{Jobs: []multiwatcher.MachineJob{multiwatcher.JobHostUnits}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *machinerSuite) TestWatch(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	result, err := s.machiner.Watch(args)
	c.Assert(err, jc.ErrorIsNil)
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
