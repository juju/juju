// Copyright 2014 Cloudbase Solutions SRL
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot_test

import (
	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/reboot"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/watcher/registry"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type machines struct {
	machine         *state.Machine
	authorizer      apiservertesting.FakeAuthorizer
	watcherRegistry facade.WatcherRegistry
	rebootAPI       *reboot.RebootAPI
	args            params.Entities

	w  state.NotifyWatcher
	wc statetesting.NotifyWatcherC
}

type rebootSuite struct {
	jujutesting.JujuConnSuite

	machine         *machines
	container       *machines
	nestedContainer *machines
}

var _ = gc.Suite(&rebootSuite{})

func (s *rebootSuite) setUpMachine(c *gc.C, machine *state.Machine) *machines {
	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming we logged in as a machine agent.
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: machine.Tag(),
	}

	watcherRegistry, err := registry.NewRegistry(clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)

	rebootAPI, err := reboot.NewRebootAPI(facadetest.Context{
		State_:           s.State,
		WatcherRegistry_: watcherRegistry,
		Auth_:            authorizer,
	})
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: machine.Tag().String()},
	}}

	resultMachine, err := rebootAPI.WatchForRebootEvent()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resultMachine.NotifyWatcherId, gc.Not(gc.Equals), "")
	c.Check(resultMachine.Error, gc.IsNil)

	resourceMachine, err := watcherRegistry.Get(resultMachine.NotifyWatcherId)
	c.Check(err, jc.ErrorIsNil)
	c.Check(resourceMachine, gc.NotNil)

	w := resourceMachine.(state.NotifyWatcher)
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertNoChange()

	return &machines{
		machine:         machine,
		authorizer:      authorizer,
		watcherRegistry: watcherRegistry,
		rebootAPI:       rebootAPI,
		args:            args,
		w:               w,
		wc:              wc,
	}
}

func (s *rebootSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}

	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	container, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)

	nestedContainer, err := s.State.AddMachineInsideMachine(template, container.Id(), instance.KVM)
	c.Assert(err, jc.ErrorIsNil)

	s.machine = s.setUpMachine(c, machine)
	s.container = s.setUpMachine(c, container)
	s.nestedContainer = s.setUpMachine(c, nestedContainer)
}

func (s *rebootSuite) TearDownTest(c *gc.C) {
	if s.machine.watcherRegistry != nil {
		workertest.DirtyKill(c, s.machine.watcherRegistry)
	}
	if s.machine.w != nil {
		workertest.CleanKill(c, s.machine.w)
		s.machine.wc.AssertClosed()
	}

	if s.container.watcherRegistry != nil {
		workertest.DirtyKill(c, s.machine.watcherRegistry)
	}
	if s.container.w != nil {
		workertest.CleanKill(c, s.container.w)
		s.container.wc.AssertClosed()
	}

	if s.nestedContainer.watcherRegistry != nil {
		workertest.DirtyKill(c, s.machine.watcherRegistry)
	}
	if s.nestedContainer.w != nil {
		workertest.CleanKill(c, s.nestedContainer.w)
		s.nestedContainer.wc.AssertClosed()
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *rebootSuite) TestWatchForRebootEvent(c *gc.C) {
	err := s.machine.machine.SetRebootFlag(true)
	c.Assert(err, jc.ErrorIsNil)

	s.machine.wc.AssertOneChange()
}

func (s *rebootSuite) TestRequestReboot(c *gc.C) {
	errResult, err := s.machine.rebootAPI.RequestReboot(s.machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		}})

	s.machine.wc.AssertOneChange()

	res, err := s.machine.rebootAPI.GetRebootAction(s.machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldReboot},
		}})
}

func (s *rebootSuite) TestClearReboot(c *gc.C) {
	errResult, err := s.machine.rebootAPI.RequestReboot(s.machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	s.machine.wc.AssertOneChange()

	res, err := s.machine.rebootAPI.GetRebootAction(s.machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldReboot},
		}})

	errResult, err = s.machine.rebootAPI.ClearReboot(s.machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	res, err = s.machine.rebootAPI.GetRebootAction(s.machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldDoNothing},
		}})
}

func (s *rebootSuite) TestRebootRequestFromMachine(c *gc.C) {
	// Request reboot on the root machine: all machines should see it
	// machine should reboot
	// container should shutdown
	// nested container should shutdown
	errResult, err := s.machine.rebootAPI.RequestReboot(s.machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		}})

	s.machine.wc.AssertOneChange()
	s.container.wc.AssertOneChange()
	s.nestedContainer.wc.AssertOneChange()

	res, err := s.machine.rebootAPI.GetRebootAction(s.machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldReboot},
		}})

	res, err = s.container.rebootAPI.GetRebootAction(s.container.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldShutdown},
		}})

	res, err = s.nestedContainer.rebootAPI.GetRebootAction(s.nestedContainer.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldShutdown},
		}})

	errResult, err = s.machine.rebootAPI.ClearReboot(s.machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	s.machine.wc.AssertOneChange()
	s.container.wc.AssertOneChange()
	s.nestedContainer.wc.AssertOneChange()
}

func (s *rebootSuite) TestRebootRequestFromContainer(c *gc.C) {
	// Request reboot on the container: container and nested container should see it
	// machine should do nothing
	// container should reboot
	// nested container should shutdown
	errResult, err := s.container.rebootAPI.RequestReboot(s.container.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		}})

	s.machine.wc.AssertNoChange()
	s.container.wc.AssertOneChange()
	s.nestedContainer.wc.AssertOneChange()

	res, err := s.machine.rebootAPI.GetRebootAction(s.machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldDoNothing},
		}})

	res, err = s.container.rebootAPI.GetRebootAction(s.container.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldReboot},
		}})

	res, err = s.nestedContainer.rebootAPI.GetRebootAction(s.nestedContainer.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldShutdown},
		}})

	errResult, err = s.container.rebootAPI.ClearReboot(s.container.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	s.machine.wc.AssertNoChange()
	s.container.wc.AssertOneChange()
	s.nestedContainer.wc.AssertOneChange()
}

func (s *rebootSuite) TestRebootRequestFromNestedContainer(c *gc.C) {
	// Request reboot on the container: container and nested container should see it
	// machine should do nothing
	// container should do nothing
	// nested container should reboot
	errResult, err := s.nestedContainer.rebootAPI.RequestReboot(s.nestedContainer.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		}})

	s.machine.wc.AssertNoChange()
	s.container.wc.AssertNoChange()
	s.nestedContainer.wc.AssertOneChange()

	res, err := s.machine.rebootAPI.GetRebootAction(s.machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldDoNothing},
		}})

	res, err = s.container.rebootAPI.GetRebootAction(s.container.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldDoNothing},
		}})

	res, err = s.nestedContainer.rebootAPI.GetRebootAction(s.nestedContainer.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldReboot},
		}})

	errResult, err = s.nestedContainer.rebootAPI.ClearReboot(s.nestedContainer.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	s.machine.wc.AssertNoChange()
	s.container.wc.AssertNoChange()
	s.nestedContainer.wc.AssertOneChange()
}
